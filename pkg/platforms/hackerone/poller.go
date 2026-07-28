package hackerone

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"github.com/cozyGarage/bbscope/v2/internal/utils"
	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	"github.com/cozyGarage/bbscope/v2/pkg/scope"
	"github.com/cozyGarage/bbscope/v2/pkg/whttp"
)

// apiBaseURL is the HackerOne API root. It is a package variable so tests can
// point the poller at a local httptest server.
var apiBaseURL = "https://api.hackerone.com"

// Poller adapts existing H1 code to the generic PlatformPoller interface.
type Poller struct {
	authB64 string
}

// NewPoller builds a HackerOne poller from username and API token.
func NewPoller(username, token string) *Poller {
	raw := username + ":" + token
	return &Poller{authB64: base64.StdEncoding.EncodeToString([]byte(raw))}
}

func (p *Poller) Name() string { return "h1" }

func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error {
	if cfg.Username != "" && cfg.Token != "" {
		raw := cfg.Username + ":" + cfg.Token
		p.authB64 = base64.StdEncoding.EncodeToString([]byte(raw))
	}
	return nil
}

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
	var handles []string
	currentURL := apiBaseURL + "/v1/hackers/programs?page%5Bsize%5D=100"
	const maxListRetries = 5
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		var res *whttp.WHTTPRes
		var err error
		for attempt := 1; attempt <= maxListRetries; attempt++ {
			res, err = whttp.SendHTTPRequest(&whttp.WHTTPReq{
				Method:  "GET",
				URL:     currentURL,
				Headers: []whttp.WHTTPHeader{{Name: "Authorization", Value: "Basic " + p.authB64}},
			}, nil)
			if err == nil {
				break
			}
			utils.Log.Warnf("HTTP request failed (attempt %d/%d), retrying: %v", attempt, maxListRetries, err)
			if attempt < maxListRetries {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(2 * time.Second):
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("listing programs failed after %d attempts: %w", maxListRetries, err)
		}

		if res.StatusCode != 200 {
			return nil, fmt.Errorf("fetching failed. Got status Code: %d", res.StatusCode)
		}

		for i := 0; i < int(gjson.Get(res.BodyString, "data.#").Int()); i++ {
			handle := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.handle").Str
			state := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.state").Str
			submissionState := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.submission_state").Str
			offersBounties := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.offers_bounties").Bool()

			// Private programs have state "soft_launched"
			isPrivate := state == "soft_launched"

			if submissionState != "open" {
				continue // Skip inactive programs
			}

			if opts.PrivateOnly && !isPrivate {
				continue
			}

			if opts.BountyOnly && !offersBounties {
				continue
			}

			handles = append(handles, handle)
		}

		nextURL := gjson.Get(res.BodyString, "links.next").Str
		if nextURL == "" {
			break
		}
		allowed, err := allowSameOriginNextURL(apiBaseURL, nextURL)
		if err != nil {
			return nil, fmt.Errorf("rejecting pagination link: %w", err)
		}
		currentURL = allowed
	}
	return handles, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	pData := scope.ProgramData{Url: "https://hackerone.com/" + handle}
	currentPageURL := apiBaseURL + "/v1/hackers/programs/" + handle + "/structured_scopes?page%5Bnumber%5D=1&page%5Bsize%5D=100"
	categoryStrings := scope.GetAllStringsForCategories(opts.Categories)

	for {
		var res *whttp.WHTTPRes
		var err error
		retries := 3
		var statusCode int

		for retries > 0 {
			res, err = whttp.SendHTTPRequest(&whttp.WHTTPReq{
				Method:  "GET",
				URL:     currentPageURL,
				Headers: []whttp.WHTTPHeader{{Name: "Authorization", Value: "Basic " + p.authB64}},
			}, nil)

			if err == nil && strings.Contains(res.BodyString, "\"data\":") {
				statusCode = res.StatusCode
				break
			}
			retries--
			time.Sleep(2 * time.Second)
		}

		if retries == 0 {
			return scope.ProgramData{}, fmt.Errorf("failed to retrieve data for %s after 3 attempts with status %d", handle, statusCode)
		}

		assetCount := int(gjson.Get(res.BodyString, "data.#").Int())
		isDumpAll := categoryStrings == nil

		for i := 0; i < assetCount; i++ {
			assetCategory := strings.ToLower(gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.asset_type").Str)
			catFound := isDumpAll
			if !isDumpAll {
				for _, cat := range categoryStrings {
					if cat == assetCategory {
						catFound = true
						break
					}
				}
			}

			if catFound {
				eligibleForBounty := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.eligible_for_bounty").Bool()
				eligibleForSubmission := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.eligible_for_submission").Bool()
				instruction := strings.ReplaceAll(gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.instruction").Str, "\n", "  ")
				target := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.asset_identifier").Str

				if eligibleForSubmission {
					if !opts.BountyOnly || eligibleForBounty {
						pData.InScope = append(pData.InScope, scope.ScopeElement{
							Target:      target,
							Description: instruction,
							Category:    assetCategory,
						})
					}
				} else {
					pData.OutOfScope = append(pData.OutOfScope, scope.ScopeElement{
						Target:      target,
						Description: instruction,
						Category:    assetCategory,
					})
				}
			}
		}

		if opts.BountyOnly && len(pData.InScope) == 0 {
			pData.OutOfScope = []scope.ScopeElement{}
		}

		nextPageURL := gjson.Get(res.BodyString, "links.next").Str
		if nextPageURL == "" {
			break
		}
		allowed, err := allowSameOriginNextURL(apiBaseURL, nextPageURL)
		if err != nil {
			return scope.ProgramData{}, fmt.Errorf("rejecting pagination link: %w", err)
		}
		currentPageURL = allowed
	}
	return pData, nil
}

// allowSameOriginNextURL ensures pagination links stay on the HackerOne API host
// so Basic auth credentials are never sent to an unexpected origin.
func allowSameOriginNextURL(base, next string) (string, error) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid API base URL: %w", err)
	}
	nextURL, err := url.Parse(next)
	if err != nil {
		return "", fmt.Errorf("invalid pagination URL: %w", err)
	}
	if !nextURL.IsAbs() {
		nextURL = baseURL.ResolveReference(nextURL)
	}
	if !strings.EqualFold(nextURL.Scheme, "https") && !strings.EqualFold(nextURL.Scheme, "http") {
		return "", fmt.Errorf("unsupported pagination URL scheme %q", nextURL.Scheme)
	}
	// httptest tests and some proxies use http; production apiBaseURL is https.
	// Always require host to match the configured API host.
	if !strings.EqualFold(nextURL.Host, baseURL.Host) {
		return "", fmt.Errorf("pagination URL host %q does not match API host %q", nextURL.Host, baseURL.Host)
	}
	return nextURL.String(), nil
}
