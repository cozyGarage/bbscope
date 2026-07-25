package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	devplatform "github.com/cozyGarage/bbscope/v2/pkg/platforms/dev"
)

// poll it: dev platform
var pollDevCmd = &cobra.Command{
	Use:    "dev",
	Short:  "Poll sample programs - testing only",
	Hidden: true,
	RunE: func(cmd *cobra.Command, _ []string) error {

		poller := devplatform.NewPoller()
		if err := poller.Authenticate(cmd.Context(), platforms.AuthConfig{}); err != nil {
			return err
		}

		return runPollWithPollers(cmd, []platforms.PlatformPoller{poller})
	},
}

func init() {
	pollCmd.AddCommand(pollDevCmd)
}
