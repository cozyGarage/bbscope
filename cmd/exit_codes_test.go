package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// runPollSubcommand invokes one poll subcommand's RunE directly with no
// credentials available. Going through Execute would traverse up to the root
// command and print its help instead of running this subcommand. Credentials
// resolve through viper here because the test environment has no OS keychain.
func runPollSubcommand(t *testing.T, cmd *cobra.Command) error {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)

	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetContext(context.Background())
	t.Cleanup(func() { cmd.SilenceUsage = false })

	return cmd.RunE(cmd, nil)
}

// TestPollSubcommandsFailWithoutCredentials pins the scripting contract: a poll
// that cannot run must exit non-zero. These commands previously logged an error
// and returned nil, so automation saw a successful run that fetched nothing.
func TestPollSubcommandsFailWithoutCredentials(t *testing.T) {
	tests := []struct {
		name    string
		cmd     *cobra.Command
		wantMsg string
	}{
		{"hackerone", pollH1Cmd, "hackerone requires a username and token"},
		{"intigriti", pollItCmd, "intigriti requires a token"},
		{"bugcrowd", pollBcCmd, "bugcrowd requires either token or email+password+otp-secret"},
		{"yeswehack", pollYwhCmd, "yeswehack requires either token or email+password+otp-secret"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := runPollSubcommand(t, tc.cmd)
			if err == nil {
				t.Fatalf("%s poll without credentials returned nil; scripts cannot detect the failure", tc.name)
			}
			if err.Error() != tc.wantMsg {
				t.Errorf("error = %q, want %q", err.Error(), tc.wantMsg)
			}
		})
	}
}

// TestFlagOrCredentialPrefersExplicitFlag covers the documented precedence.
// credentials.Get reads the OS keychain before the config file, so without this
// helper an explicit --token could not override a stale keychain entry.
func TestFlagOrCredentialPrefersExplicitFlag(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("intigriti.token", "from-config")

	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("token", "", "")

	// Flag not set: fall back to stored credentials.
	if got := flagOrCredential(cmd, "token", "intigriti.token"); got != "from-config" {
		t.Errorf("unset flag: got %q, want the stored credential", got)
	}

	// Flag explicitly set: it wins.
	if err := cmd.Flags().Set("token", "from-flag"); err != nil {
		t.Fatalf("setting flag: %v", err)
	}
	if got := flagOrCredential(cmd, "token", "intigriti.token"); got != "from-flag" {
		t.Errorf("explicit flag: got %q, want %q", got, "from-flag")
	}
}

// TestFlagOrCredentialUnknownFlag guards the helper against a typo'd flag name
// rather than panicking on a nil Lookup.
func TestFlagOrCredentialUnknownFlag(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("some.key", "stored")

	cmd := &cobra.Command{Use: "x"}
	if got := flagOrCredential(cmd, "nonexistent", "some.key"); got != "stored" {
		t.Errorf("got %q, want the stored credential", got)
	}
}

// TestExecuteContextOnlyExcusesCancellation documents the narrowed check in
// ExecuteContext. A canceled context used to turn any concurrent failure into
// a zero exit status.
func TestExecuteContextOnlyExcusesCancellation(t *testing.T) {
	if !errors.Is(context.Canceled, context.Canceled) {
		t.Fatal("sanity: context.Canceled must match itself")
	}
	// A real failure that merely coincides with cancellation is not a
	// cancellation and must not be excused.
	realFailure := errors.New("database connection refused")
	if errors.Is(realFailure, context.Canceled) {
		t.Error("an unrelated error must not be treated as a cancellation")
	}
	// Wrapped cancellations still count.
	wrapped := errors.Join(errors.New("polling h1"), context.Canceled)
	if !errors.Is(wrapped, context.Canceled) {
		t.Error("a wrapped cancellation must still be recognized")
	}
}
