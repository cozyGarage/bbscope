package cmd

import "github.com/spf13/cobra"

var getURLsCmd = &cobra.Command{
	Use:   "urls",
	Short: "Get all targets that are URLs",
	RunE: func(cmd *cobra.Command, args []string) error {
		platform, _ := cmd.Flags().GetString("platform")
		return getAndPrintTargets(cmd.Context(), "urls", platform, false)
	},
}

func init() {
	getURLsCmd.Flags().String("platform", "all", "Limit results to a specific platform (e.g. h1, bugcrowd, intigriti).")
	getCmd.AddCommand(getURLsCmd)
}
