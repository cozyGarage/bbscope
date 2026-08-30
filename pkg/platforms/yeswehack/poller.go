package yeswehack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/tidwall/gjson"

	"github.com/cozyGarage/bbscope/v2/pkg/otp"
	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	"github.com/cozyGarage/bbscope/v2/pkg/scope"
	"github.com/cozyGarage/bbscope/v2/pkg/whttp"
)

// apiBaseURL is the YesWeHack API root. It is a package variable so tests can
// point the poller at a local httptest server.
var apiBaseURL = "https://api.yeswehack.com"

type Poller struct{ token string }

func NewPoller(token string) *Poller { return &Poller{token: token} }

func (p *Poller) Name() string { return "ywh" }

func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if cfg.Token != "" {
		p.token = cfg.Token
		return nil
	}
	if cfg.Email != "" && cfg.Password != "" && cfg.OtpSecret != "" {
		tok, err := login(cfg.Email, cfg.Password, cfg.OtpSecret, cfg.Proxy)
		if err != nil {
			return err
		}
		p.token = tok
		return nil
	}
	return nil
}

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
	var handles []string
	var page = 1
	var nb_pages = 2 // Init with a value > page

	for page <= nb_pages {
		res, err := whttp.SendHTTPRequest(&whttp.WHTTPReq{
			Method:  "GET",
			URL:     apiBaseURL + "/programs" + "?page=" + strconv.Itoa(page),
			Headers: []whttp.WHTTPHeader{{Name: "Authorization", Value: "Bearer " + p.token}},
		}, nil)

		if err != nil {
			return nil, err
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return nil, fmt.Errorf("yeswehack: listing programs failed with status %d", res.StatusCode)
		}
		if !gjson.Get(res.BodyString, "items").Exists() {
			return nil, fmt.Errorf("yeswehack: listing response missing items")
		}
		if page == 1 && !gjson.Get(res.BodyString, "pagination.nb_pages").Exists() {
			return nil, fmt.Errorf("yeswehack: listing response missing pagination.nb_pages")
		}

		// Read each item as an object rather than zipping parallel `items.#.field`
		// arrays: gjson omits absent fields instead of emitting null, so a single
		// program missing an optional flag makes those arrays ragged and any
		// positional index panics.
		for _, item := range gjson.Get(res.BodyString, "items").Array() {
			slug := item.Get("slug").String()
			if slug == "" {
				continue
			}
			if item.Get("disabled").Bool() {
				continue
			}
			if opts.PrivateOnly && item.Get("public").Bool() {
				continue
			}
			if opts.BountyOnly && !item.Get("bounty").Bool() {
				continue
			}
			handles = append(handles, slug)
		}

		nb_pages = int(gjson.Get(res.BodyString, "pagination.nb_pages").Int())
		page++
	}

	return handles, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	escaped := url.PathEscape(handle)
	programAPIURL := apiBaseURL + "/programs/" + escaped
	programWebURL := "https://yeswehack.com/programs/" + escaped
	pData := scope.ProgramData{Url: programWebURL}

	res, err := whttp.SendHTTPRequest(&whttp.WHTTPReq{
		Method:  "GET",
		URL:     programAPIURL,
		Headers: []whttp.WHTTPHeader{{Name: "Authorization", Value: "Bearer " + p.token}},
	}, nil)

	if err != nil {
		return pData, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return pData, fmt.Errorf("yeswehack: fetching program %s failed with status %d", handle, res.StatusCode)
	}
	if !gjson.Get(res.BodyString, "scopes").Exists() {
		return pData, fmt.Errorf("yeswehack: program %s response missing scopes", handle)
	}
	if !gjson.Get(res.BodyString, "out_of_scope").Exists() {
		return pData, fmt.Errorf("yeswehack: program %s response missing out_of_scope", handle)
	}

	// Get the list of categories to filter by.
	// If nil, we'll include all categories.
	selectedCategories := scope.GetAllStringsForCategories(opts.Categories)

	// Read each scope entry as an object — see the note in ListProgramHandles
	// about ragged `scopes.#.field` arrays.
	for _, entry := range gjson.Get(res.BodyString, "scopes").Array() {
		target := entry.Get("scope").String()
		if target == "" {
			continue
		}
		scopeType := entry.Get("scope_type").String()

		// If selectedCategories is nil, it means "all" were selected, so we don't filter.
		if selectedCategories == nil {
			pData.InScope = append(pData.InScope, scope.ScopeElement{
				Target:   target,
				Category: scopeType,
			})
			continue
		}

		// Otherwise, check if the scopeType from the API is in our list of selected categories.
		catMatches := false
		for _, cat := range selectedCategories {
			if cat == scopeType {
				catMatches = true
				break
			}
		}

		if catMatches {
			pData.InScope = append(pData.InScope, scope.ScopeElement{
				Target:   target,
				Category: scopeType,
			})
		}
	}

	// Handle out of scope
	outOfScopeItems := gjson.Get(res.BodyString, "out_of_scope").Array()
	for _, item := range outOfScopeItems {
		pData.OutOfScope = append(pData.OutOfScope, scope.ScopeElement{
			Target:   item.String(),
			Category: "other",
		})
	}

	return pData, nil
}

func login(email string, password, otpSecret, proxy string) (string, error) {
	if proxy != "" {
		if err := whttp.SetupProxy(proxy); err != nil {
			return "", fmt.Errorf("failed to setup proxy: %w", err)
		}
	}

	loginURL := apiBaseURL + "/login"
	loginPayload, err := json.Marshal(map[string]string{"email": email, "password": password})
	if err != nil {
		return "", fmt.Errorf("building login payload: %w", err)
	}

	loginRes, err := whttp.SendHTTPRequest(&whttp.WHTTPReq{
		Method: "POST",
		URL:    loginURL,
		Headers: []whttp.WHTTPHeader{
			{Name: "Content-Type", Value: "application/json"},
		},
		Body: string(loginPayload),
	}, nil)

	if err != nil {
		return "", fmt.Errorf("failed to send login request: %w", err)
	}

	if loginRes.StatusCode != 200 {
		return "", fmt.Errorf("login failed with status code: %d", loginRes.StatusCode)
	}

	if directToken := gjson.Get(loginRes.BodyString, "token").String(); directToken != "" {
		return directToken, nil
	}

	totpToken := gjson.Get(loginRes.BodyString, "totp_token").String()
	if totpToken == "" {
		return "", fmt.Errorf("invalid login response: neither token nor totp_token found")
	}

	if otpSecret == "" {
		return "", fmt.Errorf("2FA is enabled but no OTP secret provided")
	}

	OTP_ATTEMPTS := 5
	for attempts := 1; attempts <= OTP_ATTEMPTS; attempts++ {
		code, err := otp.GenerateTOTP(otpSecret, time.Now())
		if err != nil {
			return "", fmt.Errorf("failed to generate TOTP: %w", err)
		}

		totpURL := apiBaseURL + "/account/totp"
		totpPayload, err := json.Marshal(map[string]string{"token": totpToken, "code": code})
		if err != nil {
			return "", fmt.Errorf("building TOTP payload: %w", err)
		}

		totpRes, err := whttp.SendHTTPRequest(&whttp.WHTTPReq{
			Method: "POST",
			URL:    totpURL,
			Headers: []whttp.WHTTPHeader{
				{Name: "Content-Type", Value: "application/json"},
			},
			Body: string(totpPayload),
		}, nil)

		if err != nil {
			return "", fmt.Errorf("failed to send TOTP request: %w", err)
		}

		if totpRes.StatusCode != 400 {
			if totpRes.StatusCode != 200 {
				return "", fmt.Errorf("TOTP verification failed with status code: %d", totpRes.StatusCode)
			}
			finalToken := gjson.Get(totpRes.BodyString, "token").String()
			if finalToken == "" {
				return "", fmt.Errorf("final token not found in TOTP response")
			}
			return finalToken, nil
		}

		time.Sleep(2 * time.Second)
		if attempts == OTP_ATTEMPTS {
			return "", fmt.Errorf("TOTP verification failed after %d attempts", OTP_ATTEMPTS)
		}
	}

	return "", fmt.Errorf("unexpected error in TOTP verification")
}
