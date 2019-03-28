package cmd

import (
	"github.com/awesomenix/azk/cmd/azk/cmd/nodepool"
	"github.com/spf13/cobra"
)

var ScaleCmd = &cobra.Command{
	Use: "scale",
}

func init() {
	RootCmd.AddCommand(ScaleCmd)
	ScaleCmd.AddCommand(nodepool.ScaleNodepoolCmd)
}
