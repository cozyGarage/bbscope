package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/cozyGarage/bbscope/v2/internal/utils"
	"github.com/cozyGarage/bbscope/v2/pkg/credentials"
	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	h1platform "github.com/cozyGarage/bbscope/v2/pkg/platforms/hackerone"
	"github.com/cozyGarage/bbscope/v2/pkg/whttp"
)

// poll h1: shorthand for --platform h1 with platform-specific flags
var pollH1Cmd = &cobra.Command{
	Use:   "h1",
	Short: "Poll HackerOne programs",
	RunE: func(cmd *cobra.Command, _ []string) error {
		// Use credentials package (checks keychain first, then config file)
		user := credentials.Get("hackerone.username")
		token := credentials.Get("hackerone.token")
		if user == "" || token == "" {
			utils.Log.Error("hackerone requires a username and token")
			utils.Log.Info("Set credentials with: bbscope config set hackerone.username <user>")
			utils.Log.Info("                      bbscope config set hackerone.token <token>")
			return nil
		}

		proxy, _ := rootCmd.Flags().GetString("proxy")
		if proxy != "" {
			whttp.SetupProxy(proxy)
		}
		poller := h1platform.NewPoller(user, token)
		return runPollWithPollers(cmd, []platforms.PlatformPoller{poller})
	},
}

func init() {
	pollCmd.AddCommand(pollH1Cmd)
	pollH1Cmd.Flags().StringP("user", "u", "", "HackerOne username")
	pollH1Cmd.Flags().StringP("token", "t", "", "HackerOne API token")
	viper.BindPFlag("hackerone.username", pollH1Cmd.Flags().Lookup("user"))
	viper.BindPFlag("hackerone.token", pollH1Cmd.Flags().Lookup("token"))
	// Reuse common flags from parent via cobra's flag inheritance
}
