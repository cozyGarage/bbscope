package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

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

// flagOrCredential prefers an explicitly-set command-line flag over stored
// credentials.
//
// credentials.Get consults the OS keychain before the config file, and the
// poll flags are bound to viper, which it only reaches as a fallback. Without
// this an explicit --token could not override a stale keychain entry, contrary
// to the documented precedence.
func flagOrCredential(cmd *cobra.Command, flagName, credentialKey string) string {
	if f := cmd.Flags().Lookup(flagName); f != nil && f.Changed {
		return f.Value.String()
	}
	return credentials.Get(credentialKey)
}

// buildPollersFromConfig constructs authenticated platform pollers from keychain/config.
// platformFilter is a set of short names (h1, bc, it, ywh, immunefi, dev). Empty/nil means all.
// Returns an error if the proxy configuration is invalid, matching the
// single-platform poll subcommands, which also treat a bad --proxy as fatal.
func buildPollersFromConfig(ctx context.Context, proxyURL string, platformFilter map[string]bool) ([]platforms.PlatformPoller, error) {
	allow := func(name string) bool {
		if len(platformFilter) == 0 {
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
		h1User := credentials.Get("hackerone.username")
		h1Token := credentials.Get("hackerone.token")
		if h1User != "" && h1Token != "" {
			pollers = append(pollers, h1platform.NewPoller(h1User, h1Token))
		} else {
			utils.Log.Info("Skipping HackerOne: credentials not found in keychain or config.")
		}
	}

	if allow("bc") {
		bcEmail := credentials.Get("bugcrowd.email")
		bcPass := credentials.Get("bugcrowd.password")
		bcOTP := credentials.Get("bugcrowd.otpsecret")
		if bcEmail != "" && bcPass != "" && bcOTP != "" {
			bcPoller := &bcplatform.Poller{}
			authCfg := platforms.AuthConfig{Email: bcEmail, Password: bcPass, OtpSecret: bcOTP, Proxy: proxyURL}
			if err := bcPoller.Authenticate(ctx, authCfg); err != nil {
				utils.Log.Errorf("Bugcrowd auth failed: %v", err)
			} else {
				pollers = append(pollers, bcPoller)
			}
		} else {
			utils.Log.Info("Skipping Bugcrowd: email, password, or otpsecret not found in keychain or config.")
		}
	}

	if allow("it") {
		itToken := credentials.Get("intigriti.token")
		if itToken != "" {
			itPoller := itplatform.NewPoller()
			if err := itPoller.Authenticate(ctx, platforms.AuthConfig{Token: itToken, Proxy: proxyURL}); err != nil {
				utils.Log.Errorf("Intigriti auth failed: %v", err)
			} else {
				pollers = append(pollers, itPoller)
			}
		} else {
			utils.Log.Info("Skipping Intigriti: token not found in keychain or config.")
		}
	}

	if allow("ywh") {
		ywhEmail := credentials.Get("yeswehack.email")
		ywhPass := credentials.Get("yeswehack.password")
		ywhOTP := credentials.Get("yeswehack.otpsecret")
		if ywhEmail != "" && ywhPass != "" && ywhOTP != "" {
			ywhPoller := &ywhplatform.Poller{}
			authCfg := platforms.AuthConfig{Email: ywhEmail, Password: ywhPass, OtpSecret: ywhOTP, Proxy: proxyURL}
			if err := ywhPoller.Authenticate(ctx, authCfg); err != nil {
				utils.Log.Errorf("YesWeHack auth failed: %v", err)
			} else {
				pollers = append(pollers, ywhPoller)
			}
		} else {
			utils.Log.Info("Skipping YesWeHack: email, password, or otpsecret not found in keychain or config.")
		}
	}

	if allow("immunefi") {
		pollers = append(pollers, &implatform.Poller{})
	}

	if allow("dev") {
		pollers = append(pollers, &devplatform.Poller{})
	}

	return pollers, nil
}

// parsePlatformFilter converts a comma-separated platforms flag into a filter set.
// "all" or empty returns nil (meaning no filter / all platforms).
func parsePlatformFilter(input string) map[string]bool {
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
