package bugcrowd

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/tidwall/gjson"

	"github.com/cozyGarage/bbscope/v2/internal/utils"
	"github.com/cozyGarage/bbscope/v2/pkg/otp"
	"github.com/cozyGarage/bbscope/v2/pkg/scope"
	"github.com/cozyGarage/bbscope/v2/pkg/whttp"
)

const (
	USER_AGENT               = "Mozilla/5.0 (X11; Linux x86_64; rv:82.0) Gecko/20100101 Firefox/82.0"
	RATE_LIMIT_SLEEP_SECONDS = 5

	WAF_BANNED_ERROR = "you are temporarily WAF banned, change IP or wait a few hours"
)

// apiBaseURL is the Bugcrowd site root. It is a package variable so tests can
// point listing/scope helpers at a local httptest server.
var apiBaseURL = "https://bugcrowd.com"

// rateLimitInterval is the minimum delay between rate-limited HTTP requests.
// Tests may set this to 0 to avoid sleeping against httptest.
var rateLimitInterval = 1 * time.Second

// Rate-limiting types and global channel
type rateLimitedResult struct {
	res *whttp.WHTTPRes
	err error
}

type rateLimitedRequest struct {
	req        *whttp.WHTTPReq
	client     *retryablehttp.Client // Can be nil
	resultChan chan rateLimitedResult
}

var rateLimitRequestChan chan rateLimitedRequest

func init() {
	// Initialize the rate-limited request channel and start the worker
	rateLimitRequestChan = make(chan rateLimitedRequest)
	go rateLimitedRequestWorker()
}

func rateLimitedRequestWorker() {
	// One request per second by default (otherwise Bugcrowd WAF bans us).
	// Interval is read each iteration so tests can disable the delay.
	for r := range rateLimitRequestChan {
		if interval := rateLimitInterval; interval > 0 {
			time.Sleep(interval)
		}
		res, err := whttp.SendHTTPRequest(r.req, r.client)
		r.resultChan <- rateLimitedResult{res: res, err: err}
	}
}

// rateLimitedSendHTTPRequest is a wrapper for whttp.SendHTTPRequest that enforces the 1-req/sec rate limit.
func rateLimitedSendHTTPRequest(req *whttp.WHTTPReq, client *retryablehttp.Client) (*whttp.WHTTPRes, error) {
	resultChan := make(chan rateLimitedResult, 1)
	rateLimitRequestChan <- rateLimitedRequest{
		req:        req,
		client:     client,
		resultChan: resultChan,
	}
	result := <-resultChan
	return result.res, result.err
}

// Automated email + password login. 2FA needs to be disabled
func Login(email, password, otpSecret, proxy string) (string, error) {
	// Create a cookie jar
	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", err
	}

	// Create a retryablehttp client
	retryClient := retryablehttp.NewClient()
	retryClient.Logger = log.New(io.Discard, "", 0)
	retryClient.RetryMax = 5 // Set your retry policy
	retryClient.HTTPClient.Jar = jar

	// Set proxy for custom client
	if proxy != "" {
		// Parse the proxy URL
		var proxyURL *url.URL
		proxyURL, err = url.Parse(proxy)
		if err != nil {
			return "", fmt.Errorf("invalid proxy URL: %w", err)
		}

		// Apply proxy settings directly to this client
		// Note: InsecureSkipVerify is only enabled when using a proxy for debugging
		retryClient.HTTPClient.Transport = &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		}

		// Also update the global client for other requests
		if err = whttp.SetupProxy(proxy); err != nil {
			return "", err
		}
	}

	firstRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    "https://identity.bugcrowd.com/login?user_hint=researcher&returnTo=/dashboard",
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
			},
		}, retryClient)

	if err != nil {
		return "", err
	}

	if err := bugcrowdStatusError(firstRes.StatusCode, "login page"); err != nil {
		return "", err
	}

	identityUrl, _ := url.Parse("https://identity.bugcrowd.com")
	csrfToken := ""
	for _, cookie := range retryClient.HTTPClient.Jar.Cookies(identityUrl) {
		if cookie.Name == "csrf-token" {
			csrfToken = cookie.Value
			break
		}
	}

	// Step 1: Initial login with username/password (without OTP)
	firstLoginRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "POST",
			URL:    "https://identity.bugcrowd.com/login",
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "X-Csrf-Token", Value: csrfToken},
				{Name: "Content-Type", Value: "application/x-www-form-urlencoded; charset=UTF-8"},
				{Name: "Origin", Value: "https://identity.bugcrowd.com"},
			},
			Body: "username=" + url.QueryEscape(email) + "&password=" + url.QueryEscape(password) + "&otp_code=&backup_otp_code=&user_type=RESEARCHER&remember_me=true",
		}, retryClient)

	if err != nil {
		return "", err
	}

	if err := bugcrowdStatusError(firstLoginRes.StatusCode, "login"); err != nil {
		return "", err
	}

	needsMfa := gjson.Get(firstLoginRes.BodyString, "needsMfa").Bool()
	if !needsMfa {
		return "", errors.New("unexpected response: MFA should be required")
	}

	otpCode, err := otp.GenerateTOTP(otpSecret, time.Now())
	if err != nil {
		return "", fmt.Errorf("failed to generate TOTP: %w", err)
	}

	if otpCode == "" {
		return "", fmt.Errorf("2FA code is empty")
	}

	csrfToken = ""
	for _, cookie := range retryClient.HTTPClient.Jar.Cookies(identityUrl) {
		if cookie.Name == "csrf-token" {
			csrfToken = cookie.Value
			break
		}
	}

	// Step 2: Submit OTP
	otpRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "POST",
			URL:    "https://identity.bugcrowd.com/auth/otp-challenge",
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "X-Csrf-Token", Value: csrfToken},
				{Name: "Content-Type", Value: "application/x-www-form-urlencoded; charset=UTF-8"},
				{Name: "Origin", Value: "https://identity.bugcrowd.com"},
			},
			Body: "username=" + url.QueryEscape(email) + "&password=" + url.QueryEscape(password) + "&otp_code=" + otpCode + "&backup_otp_code=&user_type=RESEARCHER&remember_me=true",
		}, retryClient)

	if err != nil {
		return "", err
	}

	if err := bugcrowdStatusError(otpRes.StatusCode, "OTP challenge"); err != nil {
		return "", err
	}

	// Check if OTP failed
	needsMfa = gjson.Get(otpRes.BodyString, "needsMfa").Bool()
	message := gjson.Get(otpRes.BodyString, "message").String()
	if needsMfa {
		return "", fmt.Errorf("2FA verification failed: %s", message)
	}

	redirectUrl := gjson.Get(otpRes.BodyString, "redirect_to").String()
	if redirectUrl == "" {
		return "", errors.New("no redirect URL found in response")
	}
	redirectUrl, err = sanitizeBugcrowdRedirectURL(redirectUrl)
	if err != nil {
		return "", err
	}

	redirectRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    redirectUrl,
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Origin", Value: "https://identity.bugcrowd.com"},
			},
		}, retryClient)

	if err != nil {
		return "", err
	}

	if err := bugcrowdStatusError(redirectRes.StatusCode, "post-login redirect"); err != nil {
		return "", err
	}

	bugcrowdUrl, _ := url.Parse("https://bugcrowd.com")
	for _, origin := range []*url.URL{identityUrl, bugcrowdUrl} {
		for _, cookie := range retryClient.HTTPClient.Jar.Cookies(origin) {
			if cookie.Name == "_bugcrowd_session" {
				utils.Log.Info("Login OK. Fetching programs, please wait...")
				return cookie.Value, nil
			}
		}
	}

	return "", errors.New("unable to obtain session cookie")
}

// validateBugcrowdRedirectURL restricts post-login redirects to bugcrowd.com hosts
// so session cookies are not sent to an attacker-controlled URL.
func validateBugcrowdRedirectURL(raw string) error {
	_, err := sanitizeBugcrowdRedirectURL(raw)
	return err
}

// sanitizeBugcrowdRedirectURL resolves relative redirects against the Bugcrowd
// identity origin and rejects any absolute URL that is not on *.bugcrowd.com over HTTPS.
func sanitizeBugcrowdRedirectURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid redirect URL: %w", err)
	}

	if u.Scheme == "" && u.Host == "" {
		// Path-relative redirect from the identity login flow.
		if u.Path == "" || !strings.HasPrefix(u.Path, "/") {
			return "", fmt.Errorf("relative redirect must be an absolute path, got %q", raw)
		}
		base, _ := url.Parse("https://identity.bugcrowd.com")
		u = base.ResolveReference(u)
	}

	if u.Scheme != "https" {
		return "", fmt.Errorf("redirect URL must use https, got %q", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host == "bugcrowd.com" || strings.HasSuffix(host, ".bugcrowd.com") {
		return u.String(), nil
	}
	return "", fmt.Errorf("redirect URL host %q is not an allowed bugcrowd.com host", host)
}

func bugcrowdStatusError(status int, what string) error {
	if status == 403 || status == 406 {
		return errors.New(WAF_BANNED_ERROR)
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("bugcrowd: %s failed with status %d", what, status)
	}
	return nil
}

// resolveBugcrowdAPIURL joins a path (or absolute URL) onto apiBaseURL and
// refuses anything that would send the session cookie off-origin. Paths in
// HTML/JSON used to be concatenated as apiBaseURL+path, so a scheme-relative
// or absolute value could leave bugcrowd.com.
func resolveBugcrowdAPIURL(pathOrURL string) (string, error) {
	pathOrURL = strings.TrimSpace(pathOrURL)
	if pathOrURL == "" {
		return "", fmt.Errorf("empty bugcrowd URL")
	}
	base, err := url.Parse(strings.TrimRight(apiBaseURL, "/") + "/")
	if err != nil {
		return "", fmt.Errorf("invalid apiBaseURL: %w", err)
	}
	ref, err := url.Parse(pathOrURL)
	if err != nil {
		return "", fmt.Errorf("invalid bugcrowd URL %q: %w", pathOrURL, err)
	}
	if ref.User != nil {
		return "", fmt.Errorf("refusing bugcrowd URL with userinfo")
	}
	resolved := base.ResolveReference(ref)
	if !strings.EqualFold(resolved.Scheme, base.Scheme) || !strings.EqualFold(resolved.Host, base.Host) {
		return "", fmt.Errorf("refusing off-origin bugcrowd URL %q", resolved.String())
	}
	return resolved.String(), nil
}

func GetProgramHandles(sessionToken string, engagementType string, pvtOnly bool) ([]string, error) {
	pageIndex := 1
	var totalCount int
	totalCountKnown := false
	paths := []string{}
	fetchedPrograms := make(map[string]bool)
	allHandlersFoundCounter := 0

	listEndpointURL := apiBaseURL + "/engagements.json?category=" + engagementType + "&sort_by=promoted&sort_direction=desc&page="

	for {
		var res *whttp.WHTTPRes
		var err error

		res, err = rateLimitedSendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "GET",
				URL:    listEndpointURL + strconv.Itoa(pageIndex),
				Headers: []whttp.WHTTPHeader{
					{Name: "Cookie", Value: "_bugcrowd_session=" + sessionToken},
					{Name: "User-Agent", Value: USER_AGENT},
				},
			}, nil)

		if err != nil {
			return nil, err
		}

		if err := bugcrowdStatusError(res.StatusCode, "listing engagements"); err != nil {
			return nil, err
		}

		result := gjson.Get(res.BodyString, "engagements")
		if !result.Exists() {
			return nil, fmt.Errorf("bugcrowd: listing response missing engagements array")
		}
		if !totalCountKnown {
			if tc := gjson.Get(res.BodyString, "paginationMeta.totalCount"); tc.Exists() {
				totalCount = int(tc.Int())
				totalCountKnown = true
			}
		}

		// If the engagements array is empty, it means there are no more programs to fetch on subsequent pages.
		if len(result.Array()) == 0 {
			break
		}

		// Iterating over each element in the programs array
		result.ForEach(func(key, value gjson.Result) bool {
			programURL := strings.TrimSpace(value.Get("briefUrl").String())
			accessStatus := value.Get("accessStatus").String()

			// Maintain a counter of unique program URLs found so paging can
			// finish even when a row has no briefUrl (those are not appended).
			if !fetchedPrograms[programURL] {
				allHandlersFoundCounter++
				fetchedPrograms[programURL] = true

				if programURL == "" {
					return true
				}
				if !pvtOnly || (pvtOnly && accessStatus != "open") {
					paths = append(paths, programURL)
				}
			}

			// Return true to continue iterating
			return true
		})

		// Print the number of programs fetched so far
		// utils.Log.Info("Fetched programs: ", len(paths), " | Total unique programs found: ", allHandlersFoundCounter)

		pageIndex++

		// Check if we have fetched all programs using allHandlersFoundCounter
		if totalCountKnown && allHandlersFoundCounter >= totalCount {
			break
		}
	}

	return paths, nil
}

func GetProgramScope(handle string, categories string, token string) (pData scope.ProgramData, err error) {
	isEngagement := strings.HasPrefix(handle, "/engagements/")
	if isEngagement {
		handle = strings.TrimPrefix(handle, "/engagements/")
	}

	pData.Url = apiBaseURL + "/" + strings.TrimPrefix(handle, "/")

	if isEngagement {
		var getBriefVersionDocument string
		getBriefVersionDocument, err = getEngagementBriefVersionDocument("/engagements/"+handle, token)
		if err != nil {
			return pData, err
		}

		if getBriefVersionDocument != "" {
			err = extractScopeFromEngagement(getBriefVersionDocument, categories, token, &pData)
			if err != nil {
				return pData, err
			}
		}
	} else {
		err = extractScopeFromTargetGroups(pData.Url, categories, token, &pData)
		if err != nil {
			return pData, err
		}
	}

	return pData, nil
}

func getEngagementBriefVersionDocument(handle string, token string) (string, error) {
	pageURL, err := resolveBugcrowdAPIURL(handle)
	if err != nil {
		return "", err
	}
	res, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    pageURL,
			Headers: []whttp.WHTTPHeader{
				{Name: "Cookie", Value: "_bugcrowd_session=" + token},
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Accept", Value: "*/*"},
			},
		}, nil)

	if err != nil {
		return "", err
	}

	if res.StatusCode == 404 {
		return "", nil
	}
	if err := bugcrowdStatusError(res.StatusCode, "engagement page "+handle); err != nil {
		return "", err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(res.BodyString))
	if err != nil {
		return "", fmt.Errorf("parsing engagement page for %s: %w", handle, err)
	}

	div := doc.Find("div[data-react-class='ResearcherEngagementBrief']")

	// Get the value of the data-api-endpoints attribute
	apiEndpointsJSON, exists := div.Attr("data-api-endpoints")
	if !exists {
		if strings.Contains(res.BodyString, "ResearcherEngagementCompliance") {
			utils.Log.Warn("Compliance required! Skipping: ", pageURL)
			return "", nil
		}
		return "", fmt.Errorf("bugcrowd: engagement page missing brief endpoints for %s", handle)
	}

	path := gjson.Get(apiEndpointsJSON, "engagementBriefApi.getBriefVersionDocument").String()
	if path == "" {
		return "", nil
	}
	return path + ".json", nil
}

func extractScopeFromEngagement(getBriefVersionDocument string, categories string, token string, pData *scope.ProgramData) (err error) {
	if getBriefVersionDocument == "" || getBriefVersionDocument == ".json" {
		// Missing brief endpoint usually means compliance gate / 2FA / HTML change.
		// Do not invent sentinel targets that would pollute storage and notifications.
		utils.Log.Warn("Compliance required or brief URL missing; skipping engagement scope extraction")
		return nil
	}
	briefURL, err := resolveBugcrowdAPIURL(getBriefVersionDocument)
	if err != nil {
		return err
	}
	res, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    briefURL,
			Headers: []whttp.WHTTPHeader{
				{Name: "Cookie", Value: "_bugcrowd_session=" + token},
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Accept", Value: "*/*"},
			},
		}, nil)

	if err != nil {
		return err
	}

	if err := bugcrowdStatusError(res.StatusCode, "engagement brief"); err != nil {
		return err
	}
	scopeArray := gjson.Get(res.BodyString, "data.scope")
	if !scopeArray.Exists() {
		return fmt.Errorf("bugcrowd: engagement brief missing data.scope")
	}
	selectedCategories := scope.GetAllStringsForCategories(categories)

	// Iterate over each element of the "scope" array
	scopeArray.ForEach(func(key, value gjson.Result) bool {
		// Check if the scope element is in-scope
		inScope := value.Get("inScope").Bool()

		// Extract the "targets" array for the current scope element
		targetsArray := value.Get("targets")

		// Iterate over each object in the "targets" array
		targetsArray.ForEach(func(objectKey, objectValue gjson.Result) bool {
			// Extract the "name", "uri", "category", and "description" fields for each object
			name := objectValue.Get("name").String()
			uri := objectValue.Get("uri").String()
			category := objectValue.Get("category").String()
			description := objectValue.Get("description").String()

			if selectedCategories != nil {
				catMatches := false
				for _, selectedCat := range selectedCategories {
					if category == selectedCat {
						catMatches = true
						break
					}
				}
				if !catMatches {
					return true
				}
			}

			if uri == "" {
				uri = name
			}

			if inScope {
				pData.InScope = append(pData.InScope, scope.ScopeElement{Target: uri, Description: description, Category: category})
			} else {
				pData.OutOfScope = append(pData.OutOfScope, scope.ScopeElement{Target: uri, Description: description, Category: category})
			}

			return true
		})

		return true
	})

	return nil
}

func extractScopeFromTargetGroups(url string, categories string, token string, pData *scope.ProgramData) error {
	res, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    url + "/target_groups",
			Headers: []whttp.WHTTPHeader{
				{Name: "Cookie", Value: "_bugcrowd_session=" + token},
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Accept", Value: "*/*"},
			},
		}, nil)

	if err != nil {
		return err
	}

	if res.StatusCode == 404 {
		return nil
	}
	if err := bugcrowdStatusError(res.StatusCode, "target groups"); err != nil {
		return err
	}
	if !gjson.Get(res.BodyString, "groups").Exists() {
		return fmt.Errorf("bugcrowd: target_groups response missing groups")
	}

	noScopeTable := true
	for i, scopeTableURL := range gjson.Get(res.BodyString, "groups.#.targets_url").Array() {
		inScope := gjson.Get(res.BodyString, fmt.Sprintf("groups.%d.in_scope", i)).Bool()
		err = extractScopeFromTargetTable(scopeTableURL.String(), categories, token, pData, inScope)
		if err != nil {
			return err
		}
		noScopeTable = false
	}

	if noScopeTable {
		utils.Log.Warn("No in-scope target table found; skipping sentinel target")
	}

	return nil
}

func extractScopeFromTargetTable(scopeTableURL string, categories string, token string, pData *scope.ProgramData, inScope bool) error {
	tableURL, err := resolveBugcrowdAPIURL(scopeTableURL)
	if err != nil {
		return err
	}
	res, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    tableURL,
			Headers: []whttp.WHTTPHeader{
				{Name: "Cookie", Value: "_bugcrowd_session=" + token},
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Accept", Value: "*/*"},
			},
		}, nil)

	if err != nil {
		return err
	}

	if err := bugcrowdStatusError(res.StatusCode, "target table"); err != nil {
		return err
	}
	if !gjson.Get(res.BodyString, "targets").Exists() {
		return fmt.Errorf("bugcrowd: target table missing targets")
	}

	json := res.BodyString
	targetsCount := gjson.Get(json, "targets.#").Int()

	// Get the list of categories to filter by.
	// If nil, we'll include all categories.
	selectedCategories := scope.GetAllStringsForCategories(categories)

	for i := 0; i < int(targetsCount); i++ {
		targetPath := fmt.Sprintf("targets.%d", i)
		name := strings.TrimSpace(gjson.Get(json, targetPath+".name").String())
		uri := strings.TrimSpace(gjson.Get(json, targetPath+".uri").String())
		category := gjson.Get(json, targetPath+".category").String()
		description := gjson.Get(json, targetPath+".description").String()

		// If selectedCategories is not nil (i.e., not "all"), then we filter.
		if selectedCategories != nil {
			catMatches := false
			for _, selectedCat := range selectedCategories {
				if category == selectedCat {
					catMatches = true
					break
				}
			}
			// If no match was found, skip this target.
			if !catMatches {
				continue
			}
		}

		if uri == "" {
			uri = name
		}

		scopeElement := scope.ScopeElement{
			Target:      uri,
			Description: description,
			Category:    category,
		}

		if inScope {
			pData.InScope = append(pData.InScope, scopeElement)
		} else {
			pData.OutOfScope = append(pData.OutOfScope, scopeElement)
		}
	}

	return nil
}
