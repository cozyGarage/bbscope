package cmd

import "github.com/spf13/cobra"

var getIPsCmd = &cobra.Command{
	Use:   "ips",
	Short: "Get all targets that are IP addresses",
	RunE: func(cmd *cobra.Command, args []string) error {
		platform, _ := cmd.Flags().GetString("platform")
		return getAndPrintTargets(cmd.Context(), "ips", platform, false)
	},
}

func init() {
	getIPsCmd.Flags().String("platform", "all", "Limit results to a specific platform (e.g. h1, bugcrowd, intigriti).")
	getCmd.AddCommand(getIPsCmd)
}
