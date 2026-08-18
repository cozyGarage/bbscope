package cmd

import "github.com/spf13/cobra"

var getDomainsCmd = &cobra.Command{
	Use:   "domains",
	Short: "Get all targets that are domains (including wildcards)",
	RunE: func(cmd *cobra.Command, args []string) error {
		aggressive, _ := cmd.Flags().GetBool("aggressive")
		platform, _ := cmd.Flags().GetString("platform")
		return getAndPrintTargets(cmd.Context(), "domains", platform, aggressive)
	},
}

func init() {
	getDomainsCmd.Flags().BoolP("aggressive", "a", false, "Apply aggressive scope transformation")
	getDomainsCmd.Flags().String("platform", "all", "Limit results to a specific platform (e.g. h1, bugcrowd, intigriti).")
	getCmd.AddCommand(getDomainsCmd)
}
