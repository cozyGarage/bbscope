package ai

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cozyGarage/bbscope/v2/internal/utils"
	"github.com/cozyGarage/bbscope/v2/pkg/scope"
	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// alternationPattern matches simple example.(it|com) / example.[it|com] expansions.
var alternationPattern = regexp.MustCompile(`^([^\s(\[]+)[\(\[]([^)\]]+)[\)\]]$`)

// ProgramInfo carries minimal details that help the LLM reason about scope entries.
type ProgramInfo struct {
	ProgramURL string
	Platform   string
	Handle     string
}

// Config controls how the AI normalizer behaves.
type Config struct {
	Provider           string
	APIKey             string
	Model              string
	Endpoint           string
	MaxBatch           int
	MaxConcurrency     int
	HTTPClient         *http.Client
	Proxy              string
	InsecureSkipVerify bool // only for debugging intercepting proxies; off by default
}

// Normalizer defines the behavior required to transform raw scope targets via LLMs.
type Normalizer interface {
	NormalizeTargets(ctx context.Context, info ProgramInfo, items []storage.TargetItem) ([]storage.TargetItem, error)
}

const (
	defaultProvider = "openai"
	// Must match the ai.model default registered in cmd/root.go and the value
	// documented in the README, so a library caller that omits Model gets the
	// same model the CLI would use.
	defaultModel          = "gpt-4o-mini"
	defaultEndpoint       = "https://api.openai.com/v1/chat/completions"
	defaultMaxBatchSize   = 25
	defaultMaxConcurrency = 10
	maxAIResponseBytes    = 10 << 20 // 10 MiB
)

// NewNormalizer builds a concrete Normalizer implementation based on the provided config.
func NewNormalizer(cfg Config) (Normalizer, error) {
	cfg.Provider = strings.TrimSpace(strings.ToLower(cfg.Provider))
	if cfg.Provider == "" {
		cfg.Provider = defaultProvider
	}

	switch cfg.Provider {
	case "openai":
		return newOpenAINormalizer(cfg)
	default:
		return nil, fmt.Errorf("unsupported AI provider: %s", cfg.Provider)
	}
}

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type openAINormalizer struct {
	apiKey         string
	model          string
	endpoint       string
	maxBatchSize   int
	maxConcurrency int
	client         httpClient
}

type normalizedResult struct {
	Targets     []string
	InScope     *bool
	Category    string
	HasCategory bool
}

func newOpenAINormalizer(cfg Config) (*openAINormalizer, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, errors.New("ai normalizaton requires an API key (set ai.api_key in config or OPENAI_API_KEY)")
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}

	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	maxBatch := cfg.MaxBatch
	if maxBatch <= 0 {
		maxBatch = defaultMaxBatchSize
	}

	maxConcurrency := cfg.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = defaultMaxConcurrency
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 45 * time.Second}
	}
	if cfg.Proxy != "" {
		proxyURL, err := url.Parse(cfg.Proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid ai proxy: %w", err)
		}
		applyProxyToClient(httpClient, proxyURL, cfg.InsecureSkipVerify)
	}

	return &openAINormalizer{
		apiKey:         apiKey,
		model:          model,
		endpoint:       endpoint,
		maxBatchSize:   maxBatch,
		maxConcurrency: maxConcurrency,
		client:         httpClient,
	}, nil
}

// NormalizeTargets applies AI-powered cleanup, expanding or correcting malformed entries
// while guaranteeing that every original item is preserved.
func (n *openAINormalizer) NormalizeTargets(ctx context.Context, info ProgramInfo, items []storage.TargetItem) ([]storage.TargetItem, error) {
	if len(items) == 0 {
		return nil, nil
	}

	utils.Log.Debugf("[ai] starting normalization for %s (%s) - %d targets", info.ProgramURL, info.Handle, len(items))

	type chunkWork struct {
		index int
		start int
		end   int
		items []storage.TargetItem
	}

	var chunks []chunkWork
	for start := 0; start < len(items); start += n.maxBatchSize {
		end := start + n.maxBatchSize
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, chunkWork{
			index: len(chunks),
			start: start,
			end:   end,
			items: items[start:end],
		})
	}

	if len(chunks) == 0 {
		return nil, nil
	}

	results := make([][]storage.TargetItem, len(chunks))

	workerLimit := n.maxConcurrency
	if workerLimit <= 0 {
		workerLimit = 1
	}
	sem := make(chan struct{}, workerLimit)

	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once

	for _, chunk := range chunks {
		wg.Add(1)
		go func(c chunkWork) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			utils.Log.Debugf("[ai] normalizing chunk %d-%d (size %d)", c.start, c.end-1, len(c.items))
			chunkResult, err := n.normalizeChunk(ctx, info, c.start, c.items)
			if err != nil {
				utils.Log.Debugf("[ai] chunk %d-%d failed: %v", c.start, c.end-1, err)
				errOnce.Do(func() { firstErr = err })
				return
			}
			utils.Log.Debugf("[ai] chunk %d-%d normalized into %d targets", c.start, c.end-1, len(chunkResult))
			results[c.index] = chunkResult
		}(chunk)
	}

	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	var out []storage.TargetItem
	for _, chunkRes := range results {
		out = append(out, chunkRes...)
	}

	utils.Log.Debugf("[ai] finished normalization for %s (%s) - expanded to %d targets", info.ProgramURL, info.Handle, len(out))
	return out, nil
}

func (n *openAINormalizer) normalizeChunk(ctx context.Context, info ProgramInfo, baseID int, items []storage.TargetItem) ([]storage.TargetItem, error) {
	normalized, err := n.queryLLM(ctx, info, baseID, items)
	if err != nil {
		return nil, err
	}
	return mergeNormalized(items, baseID, normalized), nil
}

func (n *openAINormalizer) queryLLM(ctx context.Context, info ProgramInfo, baseID int, items []storage.TargetItem) (map[int]normalizedResult, error) {
	payload := llmInput{
		ProgramURL: info.ProgramURL,
		Platform:   info.Platform,
		Handle:     info.Handle,
	}

	for idx, item := range items {
		payload.Items = append(payload.Items, llmInputItem{
			ID:          baseID + idx,
			Target:      item.URI,
			Category:    item.Category,
			Description: item.Description,
			InScope:     item.InScope,
		})
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	reqBody := openAIChatRequest{
		Model: n.model,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: string(payloadJSON)},
		},
		Temperature:    0.1,
		ResponseFormat: openAIResponseFormat{Type: "json_object"},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+n.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	limitedBody := io.LimitReader(resp.Body, maxAIResponseBytes)

	if resp.StatusCode >= 300 {
		var apiErrResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(limitedBody).Decode(&apiErrResp)
		if apiErrResp.Error.Message != "" {
			return nil, fmt.Errorf("ai normalization: %s", apiErrResp.Error.Message)
		}
		return nil, fmt.Errorf("ai normalization failed with HTTP %d", resp.StatusCode)
	}

	var apiResp openAIChatResponse
	if err := json.NewDecoder(limitedBody).Decode(&apiResp); err != nil {
		return nil, err
	}

	if len(apiResp.Choices) == 0 || strings.TrimSpace(apiResp.Choices[0].Message.Content) == "" {
		return nil, errors.New("ai normalization returned an empty response")
	}

	content := strings.TrimSpace(apiResp.Choices[0].Message.Content)

	var parsed llmOutput
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, fmt.Errorf("unable to parse AI response: %w", err)
	}

	result := make(map[int]normalizedResult, len(parsed.Items))
	for _, item := range parsed.Items {
		res := normalizedResult{
			Targets: item.Normalized,
			InScope: item.InScope,
		}
		if cat := strings.ToLower(strings.TrimSpace(item.Category)); cat != "" && scope.IsUnifiedCategory(cat) {
			res.Category = cat
			res.HasCategory = true
		}
		result[item.ID] = res
	}

	return result, nil
}

var systemPrompt = buildSystemPrompt()

func buildSystemPrompt() string {
	categories := scope.UnifiedCategories()
	sort.Strings(categories)
	return fmt.Sprintf(`You are a scope normalizer. Clean messy bug bounty targets and emit consistent, structured JSON.

Allowed unified categories: %s

Context
- You receive ProgramURL, Platform, Handle, and a list of items (id, target, category, description, in_scope flag).
- Preserve meaning. Never invent targets. If unsure, return the original string unchanged.

Baseline cleanup rules
- Trim whitespace, collapse duplicate spaces, and lowercase domains.
- Expand bracket/pipe syntax: "example.(it|com)" -> "example.it", "example.com".
- Normalize URL schemes/hosts; drop redundant default ports (http:80, https:443).
- Preserve http(s) schemes plus any path/query fragments for real URLs. Do NOT strip protocol, path, or query parameters from URLs—keep them exactly as provided (after trimming whitespace) unless the text actually represents a wildcard.
- Strip obvious regex artifacts (e.g., "\d+", "(?i)") and remove trailing dots.
- Pure descriptive text with no actionable target should be returned verbatim (same category).

Wildcard handling (critical)
- Any target that implies a wildcard (starts with "*.", ends with ".*", or contains wildcard noise) must be categorized as "wildcard".
- The normalized value for wildcard targets MUST remove every "*" prefix/suffix, the scheme, and any paths. Example: "https://*.dev.*.example.com/**" -> "example.com" (category "wildcard").
- Preserve only the base registrable domain (including necessary subdomains) once cleaned. Do NOT leave "*." in normalized output; the category alone conveys wildcard semantics.

Scope intent classification (critical)
- If the text contains exclusion phrases ("OUT OF SCOPE", "OOS", "not in scope", "excluded", "test-only", etc.), force "in_scope": false regardless of original flag.
- If the text clearly states inclusion ("in scope", "eligible", "rewarded"), set "in_scope": true.
- If unclear, omit the field (let it default).

Category normalization
- Map incoming categories to the allowed unified set. If the cleaned target obviously belongs to a different category, override it.
- URLs / websites / APIs -> "url".
- Wildcards -> "wildcard" (normalized target should be the cleaned domain without "*.").
- CIDR/IP ranges -> "cidr".
- Mobile app IDs / store links -> "android" or "ios" as appropriate (keep http(s):// store URLs intact).
- Everything else follows the unified category list. Leave empty if you agree with the provided category.

Output contract (strict)
- Return ONLY JSON: {"items":[ ... ]}
- Each input id must appear exactly once.
- Each item must include:
  • "id": original integer id
  • "normalized": non-empty array of cleaned target strings (lowercase)
  • Optional "in_scope": boolean if you have high confidence
  • Optional "category": unified category if it changes
  • Optional "notes": short clarifications (never required)
- Never emit extra keys, prose, or explanations outside the JSON payload.`, strings.Join(categories, ", "))
}

type openAIChatRequest struct {
	Model          string               `json:"model"`
	Messages       []openAIMessage      `json:"messages"`
	Temperature    float64              `json:"temperature"`
	ResponseFormat openAIResponseFormat `json:"response_format"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponseFormat struct {
	Type string `json:"type"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type llmInput struct {
	ProgramURL string         `json:"program_url"`
	Platform   string         `json:"platform"`
	Handle     string         `json:"handle"`
	Items      []llmInputItem `json:"items"`
}

type llmInputItem struct {
	ID          int    `json:"id"`
	Target      string `json:"target"`
	Category    string `json:"category"`
	Description string `json:"description,omitempty"`
	InScope     bool   `json:"in_scope"`
}

type llmOutput struct {
	Items []llmOutputItem `json:"items"`
}

type llmOutputItem struct {
	ID         int      `json:"id"`
	Normalized []string `json:"normalized"`
	InScope    *bool    `json:"in_scope,omitempty"`
	Notes      string   `json:"notes"`
	Category   string   `json:"category"`
}

func mergeNormalized(items []storage.TargetItem, baseID int, normalized map[int]normalizedResult) []storage.TargetItem {
	if len(items) == 0 {
		return nil
	}

	out := make([]storage.TargetItem, 0, len(items))
	for idx, original := range items {
		id := baseID + idx
		result := normalized[id]
		targets := sanitizeTargets(result.Targets)

		cloned := original
		cloned.Variants = nil

		if len(targets) > 0 {
			cloned.Variants = make([]storage.TargetVariant, 0, len(targets))
			baseNormalized := strings.ToLower(strings.TrimSpace(storage.NormalizeTarget(cloned.URI)))

			for _, target := range targets {
				if target == "" {
					continue
				}
				if !variantAllowed(cloned.URI, target) {
					continue
				}
				var hasInScope bool
				var inScopeVal bool
				if result.InScope != nil {
					hasInScope = true
					inScopeVal = *result.InScope
				}

				if baseNormalized != "" && target == baseNormalized && !hasInScope && !result.HasCategory {
					// AI produced the same normalized value without changing scope/category.
					continue
				}

				cloned.Variants = append(cloned.Variants, storage.TargetVariant{
					Value:       target,
					HasInScope:  hasInScope,
					InScope:     inScopeVal,
					HasCategory: result.HasCategory,
					Category:    result.Category,
				})
			}
		}

		out = append(out, cloned)
	}

	return out
}

func sanitizeTargets(targets []string) []string {
	if len(targets) == 0 {
		return nil
	}

	out := make([]string, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		trimmed := strings.TrimSpace(strings.ToLower(t))
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

// variantAllowed rejects LLM-invented targets that are not derived from the original URI.
//
// A variant may only restate or narrow the original. Widening is always
// rejected: a program that scopes one host does not authorize its parent
// domain, its siblings, or its public suffix, no matter how confidently the
// model proposes them.
func variantAllowed(original, variant string) bool {
	variant = strings.ToLower(strings.TrimSpace(variant))
	if variant == "" {
		return false
	}
	origNorm := strings.ToLower(strings.TrimSpace(storage.NormalizeTarget(original)))
	if variant == origNorm {
		return true
	}

	for _, candidate := range expandAlternationCandidates(original) {
		if candidate == variant {
			return true
		}
	}

	// Wildcards in the variant always widen (example.com ↛ *.example.com).
	if strings.Contains(variant, "*") {
		return false
	}

	vHost := variantHost(variant)
	if vHost == "" {
		return false
	}

	base := cleanScopeBase(original)
	if base == "" {
		return false
	}

	if strings.Contains(base, ".") {
		if vHost == base {
			// Restating the host of a path-scoped URL (https://example.com/api)
			// would widen to the whole origin. Wildcard-host originals like
			// https://*.example.com/** are the whole origin already.
			return !originalHasRestrictivePath(original)
		}
		if strings.HasSuffix(vHost, "."+base) {
			prefix := strings.TrimSuffix(vHost, "."+base)
			return prefix != "" && isDNSLabelPath(prefix)
		}
		return false
	}

	// Completing a right-truncated original such as "example.*": the base is a
	// bare label and the variant must resolve to exactly that label plus a
	// public suffix, so "example.com" is accepted while "evil.app" (suffix
	// ".app") and "example.com.evil.net" are not.
	if strings.HasPrefix(vHost, base+".") {
		if root, ok := storage.ExtractRootDomain(vHost); ok && root == vHost {
			return true
		}
	}
	return false
}

// variantHost extracts a hostname from a variant, ignoring scheme, path, userinfo
// and port so suffix checks cannot be fooled by http://evil.com/x.example.com.
func variantHost(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if idx := strings.IndexAny(s, "/?#"); idx >= 0 {
		s = s[:idx]
	}
	// Userinfo is not a hostname. Taking the substring after "@" let
	// evil.com@example.com pass as example.com.
	if strings.Contains(s, "@") {
		return ""
	}
	if i := strings.LastIndex(s, ":"); i >= 0 && !strings.Contains(s, "]") {
		s = s[:i]
	}
	return strings.Trim(s, ".")
}

func originalHasRestrictivePath(original string) bool {
	s := strings.ToLower(strings.TrimSpace(original))
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	idx := strings.IndexAny(s, "/?#")
	if idx < 0 {
		return false
	}
	path := s[idx:]
	switch path {
	case "/", "/*", "/**", "?", "#":
		return false
	}
	if strings.HasPrefix(path, "/**") || strings.HasPrefix(path, "/*") {
		return false
	}
	return true
}

func isDNSLabelPath(s string) bool {
	if s == "" {
		return false
	}
	for _, part := range strings.Split(s, ".") {
		if part == "" || strings.ContainsAny(part, "/*?#[]:") {
			return false
		}
	}
	return true
}

func expandAlternationCandidates(original string) []string {
	s := strings.ToLower(strings.TrimSpace(original))
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if idx := strings.IndexAny(s, "/?#"); idx >= 0 {
		s = s[:idx]
	}
	m := alternationPattern.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	prefix := m[1]
	parts := strings.Split(m[2], "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, prefix+p)
	}
	return out
}

// cleanScopeBase strips scheme/path/wildcard noise to a comparable domain-ish base.
func cleanScopeBase(original string) string {
	s := strings.ToLower(strings.TrimSpace(original))
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if idx := strings.IndexAny(s, "/?#"); idx >= 0 {
		s = s[:idx]
	}
	// Userinfo is not a hostname. Taking the substring after "@" let
	// https://evil.com@example.com/ compare as example.com.
	if strings.Contains(s, "@") {
		return ""
	}
	s = strings.ReplaceAll(s, "*.", "")
	s = strings.ReplaceAll(s, ".*", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.Trim(s, ".")
	// Drop unresolved alternation markers for base comparison.
	if i := strings.IndexAny(s, "(["); i >= 0 {
		s = strings.Trim(s[:i], ".")
	}
	return s
}

func applyProxyToClient(client *http.Client, proxyURL *url.URL, insecureSkipVerify bool) {
	var baseTransport *http.Transport
	if transport, ok := client.Transport.(*http.Transport); ok && transport != nil {
		baseTransport = transport.Clone()
	} else if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok {
		baseTransport = defaultTransport.Clone()
	} else {
		baseTransport = &http.Transport{}
	}
	baseTransport.Proxy = http.ProxyURL(proxyURL)
	if baseTransport.TLSClientConfig == nil {
		baseTransport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	} else if baseTransport.TLSClientConfig.MinVersion == 0 {
		baseTransport.TLSClientConfig.MinVersion = tls.VersionTLS12
	}
	// TLS verification stays on unless explicitly opted in for intercepting proxies.
	baseTransport.TLSClientConfig.InsecureSkipVerify = insecureSkipVerify
	client.Transport = baseTransport
}
