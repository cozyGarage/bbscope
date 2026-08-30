package cmd

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cozyGarage/bbscope/v2/internal/utils"
	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	itplatform "github.com/cozyGarage/bbscope/v2/pkg/platforms/intigriti"
	"github.com/cozyGarage/bbscope/v2/pkg/whttp"
)

// poll it: Intigriti
var pollItCmd = &cobra.Command{
	Use:   "it",
	Short: "Poll Intigriti programs",
	RunE: func(cmd *cobra.Command, _ []string) error {
		// Use credentials package (checks keychain first, then config file)
		token := flagOrCredential(cmd, "token", "intigriti.token")
		if token == "" {
			utils.Log.Info("Set credential with: bbscope config set intigriti.token <token>")
			cmd.SilenceUsage = true
			return errors.New("intigriti requires a token")
		}

		proxy, _ := rootCmd.Flags().GetString("proxy")
		if proxy != "" {
			if err := whttp.SetupProxy(proxy); err != nil {
				return err
			}
		}

		poller := itplatform.NewPoller()
		if err := poller.Authenticate(cmd.Context(), platforms.AuthConfig{Token: token, Proxy: proxy}); err != nil {
			return err
		}

		return runPollWithPollers(cmd, []platforms.PlatformPoller{poller})
	},
}

func init() {
	pollCmd.AddCommand(pollItCmd)
	pollItCmd.Flags().StringP("token", "t", "", "Intigriti authorization token (Bearer)")
	_ = viper.BindPFlag("intigriti.token", pollItCmd.Flags().Lookup("token"))
}
