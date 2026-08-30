package cmd

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	bcplatform "github.com/cozyGarage/bbscope/v2/pkg/platforms/bugcrowd"
	"github.com/cozyGarage/bbscope/v2/pkg/whttp"
)

// poll bc: Bugcrowd
var pollBcCmd = &cobra.Command{
	Use:   "bc",
	Short: "Poll Bugcrowd programs",
	RunE: func(cmd *cobra.Command, _ []string) error {
		token, _ := cmd.Flags().GetString("token") // Token is CLI-only, not from config
		// Use credentials package (checks keychain first, then config file)
		email := flagOrCredential(cmd, "email", "bugcrowd.email")
		password := flagOrCredential(cmd, "password", "bugcrowd.password")
		otpSecret := flagOrCredential(cmd, "otp-secret", "bugcrowd.otpsecret")
		proxy, _ := rootCmd.Flags().GetString("proxy")
		if proxy != "" {
			if err := whttp.SetupProxy(proxy); err != nil {
				return err
			}
		}

		// Validate auth: require either token OR (email+password+otp-secret)
		if token == "" && (email == "" || password == "" || otpSecret == "") {
			cmd.SilenceUsage = true
			return errors.New("bugcrowd requires either token or email+password+otp-secret")
		}

		poller := &bcplatform.Poller{}
		if err := poller.Authenticate(cmd.Context(), platforms.AuthConfig{Token: token, Email: email, Password: password, OtpSecret: otpSecret, Proxy: proxy}); err != nil {
			return err
		}
		return runPollWithPollers(cmd, []platforms.PlatformPoller{poller})
	},
}

func init() {
	pollCmd.AddCommand(pollBcCmd)
	pollBcCmd.Flags().StringP("token", "t", "", "Bugcrowd _bugcrowd_session cookie value")
	pollBcCmd.Flags().StringP("email", "E", "", "Bugcrowd login email")
	pollBcCmd.Flags().StringP("password", "P", "", "Bugcrowd login password")
	pollBcCmd.Flags().StringP("otp-secret", "O", "", "Bugcrowd TOTP secret (base32)")
	_ = viper.BindPFlag("bugcrowd.email", pollBcCmd.Flags().Lookup("email"))
	_ = viper.BindPFlag("bugcrowd.password", pollBcCmd.Flags().Lookup("password"))
	_ = viper.BindPFlag("bugcrowd.otpsecret", pollBcCmd.Flags().Lookup("otp-secret"))
}
