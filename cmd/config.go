package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/cozyGarage/bbscope/v2/pkg/credentials"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage bbscope configuration and credentials",
	Long: `Manage bbscope configuration and credentials.

Credentials can be stored securely in your OS keychain:
  - macOS: Keychain
  - Windows: Credential Manager
  - Linux: Secret Service (GNOME Keyring, KWallet)

Examples:
  bbscope config set hackerone.token "your-api-token"
  bbscope config get hackerone.token
  bbscope config list
  bbscope config migrate`,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Store a credential in the OS keychain",
	Long: `Store a credential securely in the OS keychain.

Available keys:
  hackerone.username    - HackerOne username
  hackerone.token       - HackerOne API token
  bugcrowd.email        - Bugcrowd email
  bugcrowd.password     - Bugcrowd password
  bugcrowd.otpsecret    - Bugcrowd 2FA secret
  intigriti.token       - Intigriti API token
  yeswehack.email       - YesWeHack email
  yeswehack.password    - YesWeHack password
  yeswehack.otpsecret   - YesWeHack 2FA secret

Example:
  bbscope config set hackerone.token "abc123xyz"`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := args[1]

		if err := credentials.Set(key, value); err != nil {
			return fmt.Errorf("failed to store credential: %w", err)
		}

		fmt.Printf("✓ Stored %s in OS keychain\n", key)
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Retrieve a credential value",
	Long: `Retrieve a credential value from keychain or config file.

The command checks the OS keychain first, then falls back to the config file.

Example:
  bbscope config get hackerone.token`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := credentials.Get(key)
		source := credentials.GetSource(key)

		if value == "" {
			// A non-zero exit lets scripts branch on "is this credential set?"
			// without parsing stdout.
			cmd.SilenceUsage = true
			return fmt.Errorf("%s not found (checked keychain and config)", key)
		}

		// Mask the value for security (show first 4 and last 4 chars)
		masked := maskValue(value)
		fmt.Printf("%s = %s (source: %s)\n", key, masked, source)
		return nil
	},
}

var configDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Remove a credential from the OS keychain",
	Long: `Remove a credential from the OS keychain.

Note: This only removes from the keychain, not from the config file.

Example:
  bbscope config delete hackerone.token`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		if err := credentials.Delete(key); err != nil {
			return fmt.Errorf("failed to delete credential: %w", err)
		}
		if credentials.Get(key) != "" {
			cmd.SilenceUsage = true
			return fmt.Errorf("%s was removed from the keychain but is still present in the config file; delete it from the config or it will keep being used", key)
		}

		fmt.Printf("✓ Deleted %s from OS keychain\n", key)
		return nil
	},
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all credential keys and their storage location",
	Long: `List all known credential keys and show where each is stored.

Sources:
  keychain - Stored securely in OS keychain
  config   - Stored in ~/.bbscope.yaml (plaintext)
  none     - Not configured`,
	RunE: func(cmd *cobra.Command, args []string) error {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "KEY\tSOURCE\tSTATUS")
		fmt.Fprintln(w, "---\t------\t------")

		for _, key := range credentials.ListKeys() {
			source := credentials.GetSource(key)
			status := "✗ Not set"
			if source == "keychain" {
				status = "✓ Secure"
			} else if source == "config" {
				status = "⚠ Plaintext"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", key, source, status)
		}
		if err := w.Flush(); err != nil {
			return fmt.Errorf("flush output: %w", err)
		}
		return nil
	},
}

var configMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate credentials from config file to OS keychain",
	Long: `Migrate all credentials from the config file to the OS keychain.

This command reads credentials from ~/.bbscope.yaml and stores them
securely in your OS keychain. The config file values are preserved
but will no longer be used once they're in the keychain.

After migration, you can manually remove the credentials from the
config file for extra security.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Migrating credentials to OS keychain...")
		fmt.Println()

		migrated := 0
		skipped := 0
		failed := 0

		for _, key := range credentials.ListKeys() {
			ok, err := credentials.MigrateToKeychain(key)
			if err != nil {
				fmt.Printf("  ✗ %s: %v\n", key, err)
				failed++
			} else if ok {
				fmt.Printf("  ✓ %s: migrated\n", key)
				migrated++
			} else {
				source := credentials.GetSource(key)
				if source == "keychain" {
					fmt.Printf("  - %s: already in keychain\n", key)
				} else {
					fmt.Printf("  - %s: not set\n", key)
				}
				skipped++
			}
		}

		fmt.Println()
		fmt.Printf("Migration complete: %d migrated, %d skipped, %d failed\n", migrated, skipped, failed)

		if migrated > 0 {
			fmt.Println()
			fmt.Println("You can now remove these credentials from ~/.bbscope.yaml")
			fmt.Println("for extra security. bbscope will use the keychain instead.")
		}

		if failed > 0 {
			cmd.SilenceUsage = true
			return fmt.Errorf("%d credential(s) could not be migrated", failed)
		}

		return nil
	},
}

func maskValue(value string) string {
	if len(value) <= 8 {
		return strings.Repeat("*", len(value))
	}
	return value[:4] + strings.Repeat("*", len(value)-8) + value[len(value)-4:]
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configDeleteCmd)
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configMigrateCmd)
}
