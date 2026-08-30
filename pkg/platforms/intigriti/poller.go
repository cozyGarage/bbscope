package intigriti

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"

	"github.com/cozyGarage/bbscope/v2/internal/utils"
	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	"github.com/cozyGarage/bbscope/v2/pkg/scope"
	"github.com/cozyGarage/bbscope/v2/pkg/whttp"
)

// apiBaseURL is the Intigriti researcher API root. It is a package variable so
// tests can point the poller at a local httptest server.
var apiBaseURL = "https://api.intigriti.com/external/researcher/v1"

type Poller struct {
	token string
	// mu guards the handle lookup maps, which are populated by
	// ListProgramHandles and read concurrently by FetchProgramScope workers.
	mu          sync.RWMutex
	urlToID     map[string]string
	handleToURL map[string]string
}

func NewPoller() *Poller {
	return &Poller{urlToID: map[string]string{}, handleToURL: map[string]string{}}
}

func (p *Poller) Name() string { return "it" }

func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if cfg.Token != "" {
		p.token = cfg.Token
	}
	return nil
}

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
	urlToID := map[string]string{}
	handleToURL := map[string]string{}
	offset := 0
	limit := 500
	total := 0
	handles := []string{}

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		res, err := whttp.SendHTTPRequest(&whttp.WHTTPReq{
			Method:  "GET",
			URL:     fmt.Sprintf(apiBaseURL+"/programs?statusId=3&limit=%d&offset=%d", limit, offset),
			Headers: []whttp.WHTTPHeader{{Name: "Authorization", Value: "Bearer " + p.token}},
		}, nil)

		if err != nil {
			return nil, err
		}

		if res.StatusCode == 401 {
			return nil, fmt.Errorf("invalid auth token")
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return nil, fmt.Errorf("intigriti: listing programs failed with status %d", res.StatusCode)
		}

		body := res.BodyString
		if offset == 0 {
			total = int(gjson.Get(body, "maxCount").Int())
		}

		records := gjson.Get(body, "records").Array()
		for _, record := range records {
			id := record.Get("id").String()
			maxBounty := record.Get("maxBounty.value").Int()
			confidentialityLevel := record.Get("confidentialityLevel.id").Int()
			programPathParts := strings.Split(record.Get("webLinks.detail").String(), "=")
			if len(programPathParts) < 2 {
				continue
			}
			programPath := programPathParts[1]
			url := "https://app.intigriti.com/researcher" + programPath

			parts := strings.Split(strings.TrimSuffix(url, "/detail"), "/")
			handle := url
			if len(parts) >= 2 {
				handle = parts[len(parts)-2] + "/" + parts[len(parts)-1]
			}

			// Filtering logic from GetAllProgramsScope
			if (opts.PrivateOnly && confidentialityLevel != 4) || !opts.PrivateOnly {
				if (opts.BountyOnly && maxBounty != 0) || !opts.BountyOnly {
					urlToID[handle] = id
					handleToURL[handle] = url
					handles = append(handles, handle)
				}
			}
		}

		offset += len(records)
		if offset >= total {
			break
		}
	}

	// Publish the freshly-built lookup maps atomically for concurrent readers.
	p.mu.Lock()
	p.urlToID = urlToID
	p.handleToURL = handleToURL
	p.mu.Unlock()
	return handles, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	p.mu.RLock()
	url := p.handleToURL[handle]
	id := p.urlToID[handle]
	p.mu.RUnlock()

	pData := scope.ProgramData{Url: url}
	if id == "" {
		// The orchestrator always calls ListProgramHandles before fetching, so a
		// missing id means this handle was not part of the listing. Skip it
		// rather than re-entering ListProgramHandles (which races the maps).
		utils.Log.Warnf("intigriti: no program id for handle %q; run ListProgramHandles first", handle)
		return pData, nil
	}

	// Fetch with bounded, context-aware retries when the API rate-limits us.
	const maxBlockedRetries = 5
	var res *whttp.WHTTPRes
	for attempt := 1; ; attempt++ {
		var err error
		res, err = whttp.SendHTTPRequest(&whttp.WHTTPReq{
			Method:  "GET",
			URL:     apiBaseURL + "/programs/" + id,
			Headers: []whttp.WHTTPHeader{{Name: "Authorization", Value: "Bearer " + p.token}},
		}, nil)
		if err != nil {
			return pData, err
		}
		if res.StatusCode == 401 {
			return pData, fmt.Errorf("invalid auth token")
		}
		if strings.Contains(res.BodyString, "Request blocked") {
			if attempt >= maxBlockedRetries {
				return pData, fmt.Errorf("intigriti: rate limited after %d attempts for %q", maxBlockedRetries, handle)
			}
			utils.Log.Info("Rate limited. Retrying...")
			select {
			case <-ctx.Done():
				return pData, ctx.Err()
			case <-time.After(2 * time.Second):
			}
			continue
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return pData, fmt.Errorf("intigriti: fetching program %s failed with status %d", handle, res.StatusCode)
		}
		break
	}

	//processed := make(map[string]struct{})
	contentArray := gjson.Get(res.BodyString, "domains.content")
	contentArray.ForEach(func(key, value gjson.Result) bool {
		endpoint := value.Get("endpoint").String()
		categoryID := value.Get("type.id").Int()
		categoryValue := value.Get("type.value").Str
		tierID := value.Get("tier.id").Int()
		description := value.Get("description").Str

		if tierID != 5 { // Not out-of-scope
			allowedCategories := getCategoryID(opts.Categories)
			if allowedCategories == nil || isInArray(int(categoryID), allowedCategories) {
				pData.InScope = append(pData.InScope, scope.ScopeElement{
					Target:      endpoint,
					Description: strings.ReplaceAll(description, "\n", "  "),
					Category:    categoryValue,
				})
			}
		} else {
			pData.OutOfScope = append(pData.OutOfScope, scope.ScopeElement{
				Target:      endpoint,
				Description: strings.ReplaceAll(description, "\n", "  "),
				Category:    categoryValue,
			})
		}
		return true
	})

	return pData, nil
}
func getCategoryID(input string) []int {
	input = strings.ToLower(input)
	if input == "all" || input == "" {
		return nil
	}

	categories := map[string][]int{
		"url":      {1},
		"cidr":     {4},
		"mobile":   {2, 3},
		"android":  {2},
		"apple":    {3},
		"device":   {5},
		"other":    {6},
		"wildcard": {7},
	}
	selected, ok := categories[input]
	if !ok {
		return nil // Default to all if category is invalid
	}
	return selected
}

func isInArray(val int, array []int) bool {
	for _, item := range array {
		if item == val {
			return true
		}
	}
	return false
}
