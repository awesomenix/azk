package cmd

import (
	"github.com/spf13/cobra"
)

var addonsCmd = &cobra.Command{
	Use:   "addons",
	Short: "Manage a Addons in Kubernetes Cluster on Azure",
	Long:  `Manage a Addons in Kubernetes Cluster on Azure with one command`,
}

var createAddonsCmd = &cobra.Command{
	Use:   "create",
	Short: "Create Addons",
	Long:  `Create Addons with one command`,
}

var deleteAddonsCmd = &cobra.Command{
	Use:   "create",
	Short: "Create Addons",
	Long:  `Create Addons with one command`,
}

var updateAddonsCmd = &cobra.Command{
	Use:   "update",
	Short: "Update Addons",
	Long:  `Update Addons with one command`,
}

func init() {
	RootCmd.AddCommand(addonsCmd)

	// Create
	createAddonsCmd.Flags().StringVarP(&cnpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	createAddonsCmd.MarkFlagRequired("subscriptionid")
	createAddonsCmd.Flags().StringVarP(&cnpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	createAddonsCmd.MarkFlagRequired("resourcegroup")

	addonsCmd.AddCommand(createAddonsCmd)

	// Delete
	deleteAddonsCmd.Flags().StringVarP(&dnpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	deleteAddonsCmd.MarkFlagRequired("subscriptionid")
	deleteAddonsCmd.Flags().StringVarP(&dnpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	deleteAddonsCmd.MarkFlagRequired("resourcegroup")

	addonsCmd.AddCommand(deleteAddonsCmd)

	// Upgrade
	updateAddonsCmd.Flags().StringVarP(&dnpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	updateAddonsCmd.MarkFlagRequired("subscriptionid")
	updateAddonsCmd.Flags().StringVarP(&dnpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	updateAddonsCmd.MarkFlagRequired("resourcegroup")

	addonsCmd.AddCommand(updateAddonsCmd)
}
