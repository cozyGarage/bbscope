package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cozyGarage/bbscope/v2/pkg/credentials"
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
