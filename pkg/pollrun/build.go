package pollrun

import (
	"context"
	"fmt"
	"strings"

	"github.com/cozyGarage/bbscope/v2/internal/utils"
	"github.com/cozyGarage/bbscope/v2/pkg/credentials"
	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	bcplatform "github.com/cozyGarage/bbscope/v2/pkg/platforms/bugcrowd"
	devplatform "github.com/cozyGarage/bbscope/v2/pkg/platforms/dev"
	h1platform "github.com/cozyGarage/bbscope/v2/pkg/platforms/hackerone"
	implatform "github.com/cozyGarage/bbscope/v2/pkg/platforms/immunefi"
	itplatform "github.com/cozyGarage/bbscope/v2/pkg/platforms/intigriti"
	ywhplatform "github.com/cozyGarage/bbscope/v2/pkg/platforms/yeswehack"
	"github.com/cozyGarage/bbscope/v2/pkg/whttp"
)

// BuildPollers constructs authenticated platform pollers from the OS keychain
// and config file.
//
// platformFilter is a set of short names (h1, bc, it, ywh, immunefi, dev); an
// empty or nil set means every real platform. A platform whose credentials are
// missing is skipped with a log line rather than failing the run, so a partial
// configuration still polls what it can. An invalid proxy is fatal, matching
// the single-platform poll subcommands.
func BuildPollers(ctx context.Context, proxyURL string, platformFilter map[string]bool) ([]platforms.PlatformPoller, error) {
	explicit := len(platformFilter) > 0
	allow := func(name string) bool {
		if !explicit {
			return true
		}
		return platformFilter[name]
	}

	if proxyURL != "" {
		if err := whttp.SetupProxy(proxyURL); err != nil {
			return nil, fmt.Errorf("failed to setup proxy: %w", err)
		}
	}

	var pollers []platforms.PlatformPoller

	if allow("h1") {
		user := credentials.Get("hackerone.username")
		token := credentials.Get("hackerone.token")
		if user != "" && token != "" {
			pollers = append(pollers, h1platform.NewPoller(user, token))
		} else {
			utils.Log.Info("Skipping HackerOne: credentials not found in keychain or config.")
		}
	}

	if allow("bc") {
		email := credentials.Get("bugcrowd.email")
		password := credentials.Get("bugcrowd.password")
		otp := credentials.Get("bugcrowd.otpsecret")
		if email != "" && password != "" && otp != "" {
			poller := &bcplatform.Poller{}
			cfg := platforms.AuthConfig{Email: email, Password: password, OtpSecret: otp, Proxy: proxyURL}
			if err := poller.Authenticate(ctx, cfg); err != nil {
				utils.Log.Errorf("Bugcrowd auth failed: %v", err)
			} else {
				pollers = append(pollers, poller)
			}
		} else {
			utils.Log.Info("Skipping Bugcrowd: email, password, or otpsecret not found in keychain or config.")
		}
	}

	if allow("it") {
		token := credentials.Get("intigriti.token")
		if token != "" {
			poller := itplatform.NewPoller()
			if err := poller.Authenticate(ctx, platforms.AuthConfig{Token: token, Proxy: proxyURL}); err != nil {
				utils.Log.Errorf("Intigriti auth failed: %v", err)
			} else {
				pollers = append(pollers, poller)
			}
		} else {
			utils.Log.Info("Skipping Intigriti: token not found in keychain or config.")
		}
	}

	if allow("ywh") {
		email := credentials.Get("yeswehack.email")
		password := credentials.Get("yeswehack.password")
		otp := credentials.Get("yeswehack.otpsecret")
		if email != "" && password != "" && otp != "" {
			poller := &ywhplatform.Poller{}
			cfg := platforms.AuthConfig{Email: email, Password: password, OtpSecret: otp, Proxy: proxyURL}
			if err := poller.Authenticate(ctx, cfg); err != nil {
				utils.Log.Errorf("YesWeHack auth failed: %v", err)
			} else {
				pollers = append(pollers, poller)
			}
		} else {
			utils.Log.Info("Skipping YesWeHack: email, password, or otpsecret not found in keychain or config.")
		}
	}

	if allow("immunefi") {
		pollers = append(pollers, &implatform.Poller{})
	}

	// The dev poller emits fixed sample data for testing and is only ever
	// included when asked for by name. Treating it as a normal platform meant a
	// plain `bbscope poll --db` wrote synthetic example.com programs into the
	// user's database alongside their real scope.
	if explicit && platformFilter["dev"] {
		pollers = append(pollers, &devplatform.Poller{})
	}

	return pollers, nil
}

// ParsePlatformFilter converts a comma-separated platforms flag into a filter
// set, accepting either the short name or the full platform name.
// "all" or empty returns nil, meaning no filter.
func ParsePlatformFilter(input string) map[string]bool {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" || input == "all" {
		return nil
	}
	out := make(map[string]bool)
	for _, part := range strings.Split(input, ",") {
		part = strings.TrimSpace(part)
		switch part {
		case "hackerone":
			part = "h1"
		case "bugcrowd":
			part = "bc"
		case "intigriti":
			part = "it"
		case "yeswehack":
			part = "ywh"
		}
		if part != "" {
			out[part] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
