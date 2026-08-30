package cmd

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	ywhplatform "github.com/cozyGarage/bbscope/v2/pkg/platforms/yeswehack"
	"github.com/cozyGarage/bbscope/v2/pkg/whttp"
)

// poll ywh: shorthand for YesWeHack
var pollYwhCmd = &cobra.Command{
	Use:   "ywh",
	Short: "Poll YesWeHack programs",
	RunE: func(cmd *cobra.Command, _ []string) error {
		token, _ := cmd.Flags().GetString("token") // Token is CLI-only, not from config
		// Use credentials package (checks keychain first, then config file)
		email := flagOrCredential(cmd, "email", "yeswehack.email")
		password := flagOrCredential(cmd, "password", "yeswehack.password")
		otpSecret := flagOrCredential(cmd, "otp-secret", "yeswehack.otpsecret")
		proxy, _ := rootCmd.Flags().GetString("proxy")
		if proxy != "" {
			if err := whttp.SetupProxy(proxy); err != nil {
				return err
			}
		}
		// Validate auth: require either token OR (email+password+otp-secret)
		if token == "" && (email == "" || password == "" || otpSecret == "") {
			cmd.SilenceUsage = true
			return errors.New("yeswehack requires either token or email+password+otp-secret")
		}

		poller := &ywhplatform.Poller{}
		if err := poller.Authenticate(cmd.Context(), platforms.AuthConfig{Token: token, Email: email, Password: password, OtpSecret: otpSecret, Proxy: proxy}); err != nil {
			return err
		}
		return runPollWithPollers(cmd, []platforms.PlatformPoller{poller})
	},
}

func init() {
	pollCmd.AddCommand(pollYwhCmd)
	pollYwhCmd.Flags().StringP("token", "t", "", "YesWeHack bearer token (optional if using email/password + otp secret)")
	pollYwhCmd.Flags().StringP("email", "E", "", "YesWeHack login email")
	pollYwhCmd.Flags().StringP("password", "P", "", "YesWeHack login password")
	pollYwhCmd.Flags().StringP("otp-secret", "O", "", "YesWeHack TOTP secret (base32)")
	_ = viper.BindPFlag("yeswehack.email", pollYwhCmd.Flags().Lookup("email"))
	_ = viper.BindPFlag("yeswehack.password", pollYwhCmd.Flags().Lookup("password"))
	_ = viper.BindPFlag("yeswehack.otpsecret", pollYwhCmd.Flags().Lookup("otp-secret"))
}
