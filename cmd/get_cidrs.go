package cmd

import "github.com/spf13/cobra"

var getCIDRsCmd = &cobra.Command{
	Use:   "cidrs",
	Short: "Get all targets that are CIDR ranges or IP ranges",
	RunE: func(cmd *cobra.Command, args []string) error {
		platform, _ := cmd.Flags().GetString("platform")
		return getAndPrintTargets(cmd.Context(), "cidrs", platform, false)
	},
}

func init() {
	getCIDRsCmd.Flags().String("platform", "all", "Limit results to a specific platform (e.g. h1, bugcrowd, intigriti).")
	getCmd.AddCommand(getCIDRsCmd)
}
