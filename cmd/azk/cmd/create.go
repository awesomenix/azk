package cmd

import (
	"github.com/awesomenix/azk/cmd/azk/cmd/addons"
	"github.com/awesomenix/azk/cmd/azk/cmd/cluster"
	"github.com/awesomenix/azk/cmd/azk/cmd/controlplane"
	"github.com/awesomenix/azk/cmd/azk/cmd/flow"
	"github.com/awesomenix/azk/cmd/azk/cmd/nodepool"
	"github.com/spf13/cobra"
)

var CreateCmd = &cobra.Command{
	Use: "create",
}

func init() {
	RootCmd.AddCommand(CreateCmd)
	CreateCmd.AddCommand(cluster.CreateClusterCmd)
	CreateCmd.AddCommand(controlplane.CreateControlPlaneCmd)
	CreateCmd.AddCommand(nodepool.CreateNodepoolCmd)
	CreateCmd.AddCommand(flow.FlowCmd)
	CreateCmd.AddCommand(addons.CreateAddonsCmd)
}
