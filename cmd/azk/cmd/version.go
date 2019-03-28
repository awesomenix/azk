package cmd

import (
	"github.com/spf13/cobra"
)

const Version = "0.0.1-alpha.4"

var VersionCmd = &cobra.Command{
	Args:  cobra.NoArgs,
	Use:   "version",
	Short: "prints the version",
	Long:  "prints the version",
	RunE: func(cmd *cobra.Command, args []string) error {
		println(Version)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(VersionCmd)
}
