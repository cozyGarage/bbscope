package notify

import (
	"net/url"
	"strings"
	"testing"
)

func FuzzWebhookURLAllowed(f *testing.F) {
	f.Add("https://example.com/hook")
	f.Add("http://127.0.0.1:8080/h")
	f.Add("http://169.254.169.254/latest/meta-data")
	f.Add("ftp://example.com")
	f.Add("")
	f.Add("://")
	f.Fuzz(func(t *testing.T, input string) {
		err := webhookURLAllowed(input)
		if err != nil {
			return
		}
		u, parseErr := url.Parse(strings.TrimSpace(input))
		if parseErr != nil {
			t.Fatalf("accepted unparseable URL %q", input)
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "http" && scheme != "https" {
			t.Fatalf("accepted scheme %q: %q", input, scheme)
		}
		if u.Host == "" || u.User != nil {
			t.Fatalf("accepted URL without host or with userinfo: %q", input)
		}
		switch strings.ToLower(u.Hostname()) {
		case "169.254.169.254", "metadata.google.internal", "metadata.google.internal.", "fd00:ec2::254":
			t.Fatalf("accepted metadata host: %q", input)
		}
	})
}
