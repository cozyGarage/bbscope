package immunefi

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	"github.com/cozyGarage/bbscope/v2/pkg/scope"
	"github.com/cozyGarage/bbscope/v2/pkg/whttp"
)

// maxRetries and sleepFunc are package variables so httptest tests can avoid
// long exponential backoff against a local server. whttp's client already
// retries 429/5xx/network errors 10 times per call (honoring Retry-After), so
// this outer loop only covers a few longer outages; 20 here meant up to ~220
// requests for one URL.
var maxRetries = 3
var sleepFunc = func(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

type Poller struct{}

func (p *Poller) Name() string { return "immunefi" }

// Authenticate is a no-op for Immunefi (no auth required)
func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error { return nil }

// fetchWithRetry sends an HTTP request, retrying network errors, 429 rate
// limits, and 5xx responses up to maxRetries times with backoff. Other 4xx
// responses fail immediately: a delisted program's 404 never recovers.
func fetchWithRetry(ctx context.Context, url string) (*whttp.WHTTPRes, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		res, err := whttp.SendHTTPRequest(
			&whttp.WHTTPReq{
				Ctx:    ctx,
				Method: "GET",
				URL:    url,
				Headers: []whttp.WHTTPHeader{
					{Name: "Accept", Value: "*/*"},
					{Name: "Rsc", Value: "1"},
				},
			}, nil)

		backoff := time.Duration(attempt+1) * time.Second
		switch {
		case err != nil:
			lastErr = err
		case res.StatusCode >= 200 && res.StatusCode < 300:
			return res, nil
		case res.StatusCode == 429:
			lastErr = fmt.Errorf("HTTP 429 for %s", url)
			backoff = min(2*backoff, 30*time.Second)
		case res.StatusCode >= 500:
			// Immunefi occasionally serves transient 5xx during RSC navigations.
			lastErr = fmt.Errorf("HTTP %d for %s", res.StatusCode, url)
		default:
			return nil, fmt.Errorf("HTTP %d for %s", res.StatusCode, url)
		}
		if err := sleepFunc(ctx, backoff); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
	res, err := fetchWithRetry(ctx, PLATFORM_URL+"/bug-bounty/")
	if err != nil {
		return nil, err
	}

	// The RSC response contains embedded JSON. Extract the bounties array.
	// Look for "bounties":[...] pattern
	bountiesRegex := regexp.MustCompile(`"bounties":\[`)
	match := bountiesRegex.FindStringIndex(res.BodyString)
	if match == nil {
		return nil, fmt.Errorf("immunefi: bounties array not found in listing response (page structure may have changed)")
	}

	// Find the matching closing bracket for the bounties array
	startIdx := match[0] + len(`"bounties":`)
	bountyJSON := extractJSONArray(res.BodyString[startIdx:])
	if bountyJSON == "" {
		return nil, fmt.Errorf("immunefi: failed to extract bounties JSON from listing response")
	}

	var programURLs []string
	jsonPrograms := gjson.Parse(bountyJSON)
	programs := jsonPrograms.Array()
	sawIdentifier := false

	for _, program := range programs {
		slug := immunefiProgramSlug(program)
		if slug != "" {
			sawIdentifier = true
		}
		if slug == "" || program.Get("inviteOnly").Bool() {
			continue
		}
		programURLs = append(programURLs, PLATFORM_URL+"/bug-bounty/"+slug+"/information/")
	}

	// Live listing objects use slug/url, not id. An array that parses but
	// yields no identifiers is a schema change, not an empty bounty list —
	// returning [] would let sync disable every stored Immunefi program.
	if len(programs) > 0 && !sawIdentifier {
		return nil, fmt.Errorf("immunefi: listing objects have no slug, url, or id (page structure may have changed)")
	}

	return programURLs, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	pData := scope.ProgramData{Url: handle}

	res, err := fetchWithRetry(ctx, handle)
	if err != nil {
		return pData, err
	}

	selectedCategories := getCategories(opts.Categories)
	includeUnknown := includeUnknownImmunefiTypes(opts.Categories)

	// Extract assets array from RSC response. A missing marker usually means the
	// page shape changed; treat that as an error so sync does not see "empty success".
	assetsRegex := regexp.MustCompile(`"assets":\[`)
	match := assetsRegex.FindStringIndex(res.BodyString)
	if match == nil {
		return pData, fmt.Errorf("immunefi: assets array not found for %s", handle)
	}

	startIdx := match[0] + len(`"assets":`)
	assetsJSON := extractJSONArray(res.BodyString[startIdx:])
	if assetsJSON == "" {
		return pData, fmt.Errorf("immunefi: failed to extract assets JSON for %s", handle)
	}

	var tempScope []scope.ScopeElement
	jsonAssets := gjson.Parse(assetsJSON)

	for _, asset := range jsonAssets.Array() {
		elementTarget := asset.Get("url").String()
		elementType := asset.Get("type").String()
		elementDesc := asset.Get("description").String()

		if cat, ok := matchImmunefiAsset(elementType, selectedCategories, includeUnknown); ok {
			tempScope = append(tempScope, scope.ScopeElement{
				Target:      elementTarget,
				Description: elementDesc,
				Category:    cat,
			})
		}
	}

	pData.InScope = tempScope
	pData.OutOfScope = nil

	return pData, nil
}

// extractJSONArray extracts a JSON array starting at position 0 of the input string.
// It handles nested brackets and returns the complete array including brackets.
func extractJSONArray(s string) string {
	if len(s) == 0 || s[0] != '[' {
		return ""
	}

	depth := 0
	inString := false
	escaped := false

	for i, c := range s {
		if escaped {
			escaped = false
			continue
		}

		if c == '\\' && inString {
			escaped = true
			continue
		}

		if c == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		if c == '[' {
			depth++
		} else if c == ']' {
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}

	return ""
}

func includeUnknownImmunefiTypes(categories string) bool {
	c := strings.ToLower(strings.TrimSpace(categories))
	return c == "" || c == "all"
}

func matchImmunefiAsset(elementType string, selected []string, includeUnknown bool) (string, bool) {
	for _, cat := range selected {
		if strings.Contains(elementType, cat) {
			return cat, true
		}
	}
	if includeUnknown {
		if elementType == "" {
			return "url", true
		}
		return elementType, true
	}
	return "", false
}

// immunefiProgramSlug prefers the fields live RSC objects actually carry
// (slug, then url). Testdata and older payloads still use id.
func immunefiProgramSlug(program gjson.Result) string {
	if slug := strings.TrimSpace(program.Get("slug").String()); slug != "" {
		return strings.Trim(slug, "/")
	}
	if raw := strings.TrimSpace(program.Get("url").String()); raw != "" {
		if slug := slugFromImmunefiURL(raw); slug != "" {
			return slug
		}
	}
	if id := strings.TrimSpace(program.Get("id").String()); id != "" {
		if strings.Contains(id, "/") {
			return slugFromImmunefiURL(id)
		}
		return strings.Trim(id, "/")
	}
	return ""
}

func slugFromImmunefiURL(raw string) string {
	raw = strings.TrimSpace(raw)
	path := raw
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			path = rest[slash:]
		} else {
			return ""
		}
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, part := range parts {
		if part == "bug-bounty" && i+1 < len(parts) && parts[i+1] != "" && parts[i+1] != "information" {
			return parts[i+1]
		}
	}
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" && parts[i] != "information" {
			return parts[i]
		}
	}
	return ""
}
