package cmd

import (
	"github.com/awesomenix/azk/cmd/addons"
	"github.com/awesomenix/azk/cmd/controlplane"
	"github.com/awesomenix/azk/cmd/nodepool"
	"github.com/spf13/cobra"
)

var UpgradeCmd = &cobra.Command{
	Use: "upgrade",
}

func init() {
	RootCmd.AddCommand(UpgradeCmd)
	UpgradeCmd.AddCommand(controlplane.UpgradeControlPlaneCmd)
	UpgradeCmd.AddCommand(nodepool.UpgradeNodepoolCmd)
	UpgradeCmd.AddCommand(addons.UpgradeAddonsCmd)
}
