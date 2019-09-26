package cmd

import (
	"github.com/awesomenix/azk/cmd/addons"
	"github.com/awesomenix/azk/cmd/cluster"
	"github.com/awesomenix/azk/cmd/nodepool"
	"github.com/spf13/cobra"
)

var DeleteCmd = &cobra.Command{
	Use: "delete",
}

func init() {
	RootCmd.AddCommand(DeleteCmd)
	DeleteCmd.AddCommand(cluster.DeleteClusterCmd)
	DeleteCmd.AddCommand(nodepool.DeleteNodepoolCmd)
	DeleteCmd.AddCommand(addons.DeleteAddonsCmd)
}
