package whttp

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"strings"
	"unicode/utf8"

	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/net/html"
)

type WHTTPHeader struct {
	Name  string
	Value string
}

type WHTTPReq struct {
	URL        string
	Method     string
	Body       string
	CustomHost string
	Headers    []WHTTPHeader
	Debug      bool
}

type WHTTPRes struct {
	StatusCode     int
	ResponseLength int
	HTTPTitle      string
	BodyString     string
	Headers        http.Header
}

var retryClient *retryablehttp.Client
var GlobalDebug bool

// maxResponseBytes caps how much of a response body we read into memory to
// avoid unbounded memory growth from very large or malicious responses.
const maxResponseBytes = 100 << 20 // 100 MiB

// sensitiveHeaders are redacted in debug output to prevent credential leakage
var sensitiveHeaders = []string{
	"authorization",
	"proxy-authorization",
	"cookie",
	"set-cookie",
	"x-csrf-token",
	"x-api-key",
	"x-auth-token",
}

func init() {
	retryClient = retryablehttp.NewClient()
	retryClient.RetryMax = 10 // Reasonable retry limit to avoid infinite loops

	// Default timeout to 30 seconds
	retryClient.HTTPClient.Timeout = 30 * time.Second

	// Configure retry timing
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 30 * time.Second

	// Don't print debug messages
	retryClient.Logger = log.New(io.Discard, "", 0)
}

// isSensitiveHeader checks if a header should be redacted in debug output
func isSensitiveHeader(name string) bool {
	nameLower := strings.ToLower(name)
	for _, h := range sensitiveHeaders {
		if nameLower == h {
			return true
		}
	}
	return false
}

// redactDebugBody replaces request/response bodies in debug output when they
// look like login/auth payloads that commonly contain secrets.
func redactDebugBody(body string) string {
	lower := strings.ToLower(body)
	sensitiveMarkers := []string{
		"password",
		"passwd",
		"secret",
		"otp",
		"totp",
		"token",
		"authorization",
		"client_secret",
		"refresh_token",
		"access_token",
		"api_key",
		"apikey",
	}
	for _, m := range sensitiveMarkers {
		if strings.Contains(lower, m) {
			return "[REDACTED BODY]"
		}
	}
	return body
}

func GetDefaultClient() *retryablehttp.Client {
	return retryClient
}

func SendHTTPRequest(wReq *WHTTPReq, customClient *retryablehttp.Client) (wRes *WHTTPRes, err error) {
	client := customClient
	if client == nil {
		client = retryClient // Use the default client
	}

	var req *retryablehttp.Request
	if wReq.Body != "" {
		req, err = retryablehttp.NewRequest(wReq.Method, wReq.URL, strings.NewReader(wReq.Body))
	} else {
		req, err = retryablehttp.NewRequest(wReq.Method, wReq.URL, nil)
	}

	if err != nil {
		return nil, err
	}

	if wReq.CustomHost != "" {
		req.Host = wReq.CustomHost
	} else {
		if strings.HasSuffix(req.Host, ":80") {
			req.Host = strings.TrimSuffix(req.Host, ":80")
		} else if strings.HasSuffix(req.Host, ":443") {
			req.Host = strings.TrimSuffix(req.Host, ":443")
		}
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:83.0) Gecko/20100101 Firefox/83.0 bbscope")
	req.Header.Set("Cache-Control", "no-transform")
	req.Header.Set("Connection", "close")
	req.Header.Set("Accept-Language", "en")

	if len(wReq.Headers) > 0 {
		for _, h := range wReq.Headers {
			req.Header.Add(h.Name, h.Value)
		}
	}

	if wReq.Debug || GlobalDebug {
		fmt.Println("********** HTTP REQUEST **********")
		fmt.Printf("%s %s\n", wReq.Method, wReq.URL)
		if req.Host != "" {
			fmt.Printf("Host: %s\n", req.Host)
		}
		for k, v := range req.Header {
			// Redact sensitive headers to prevent credential leakage
			if isSensitiveHeader(k) {
				fmt.Printf("%s: [REDACTED]\n", k)
			} else {
				fmt.Printf("%s: %s\n", k, strings.Join(v, ", "))
			}
		}
		if wReq.Body != "" {
			fmt.Printf("\n%s\n", redactDebugBody(wReq.Body))
		}
		fmt.Println("**********************************")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	wRes = &WHTTPRes{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}

	wRes.BodyString = string(bodyBytes)
	wRes.StatusCode = resp.StatusCode

	if wReq.Debug || GlobalDebug {
		fmt.Println("********** HTTP RESPONSE **********")
		fmt.Printf("Status: %d\n", resp.StatusCode)
		for k, v := range resp.Header {
			if isSensitiveHeader(k) {
				fmt.Printf("%s: [REDACTED]\n", k)
			} else {
				fmt.Printf("%s: %s\n", k, strings.Join(v, ", "))
			}
		}
		fmt.Printf("\n%s\n", redactDebugBody(wRes.BodyString))
		fmt.Println("***********************************")
	}

	if title, ok := getHTMLTitle(wRes.BodyString); ok {
		wRes.HTTPTitle = strings.ToValidUTF8(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(title, "\n", ""), "\r", "")), "")
	}

	wRes.ResponseLength = utf8.RuneCountInString(wRes.BodyString)
	return wRes, nil
}

func SetupProxy(proxyURL string) error {
	if proxyURL == "" {
		return nil
	}

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("invalid proxy URL: %w", err)
	}

	client := GetDefaultClient()
	client.HTTPClient.Transport = &http.Transport{
		Proxy: http.ProxyURL(parsedURL),
		TLSClientConfig: &tls.Config{
			// Intercepting proxies (Burp/ZAP) present their own certificate, so
			// verification is skipped here. This transport is only installed when
			// the user explicitly passes --proxy. Use a modern TLS floor and let
			// Go negotiate cipher suites instead of pinning obsolete TLS 1.1.
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
		},
	}

	return nil
}

func isTitleElement(n *html.Node) bool {
	return n.Type == html.ElementNode && n.Data == "title"
}

func traverse(n *html.Node) (string, bool) {
	if isTitleElement(n) {
		if n.FirstChild != nil {
			return n.FirstChild.Data, true
		}
		return "", true
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		result, ok := traverse(c)
		if ok {
			return result, ok
		}
	}

	return "", false
}

func getHTMLTitle(requestBody string) (string, bool) {
	doc, err := html.Parse(strings.NewReader(requestBody))
	if err != nil {
		fmt.Println("Failed to parse HTML!")
		return "", true
	}

	return traverse(doc)
}
