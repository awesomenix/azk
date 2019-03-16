package cmd

import (
	"io/ioutil"
	"os"

	"github.com/awesomenix/azk/assets/addons"
	"github.com/spf13/cobra"
)

var (
	// SupportedAddons list of supported addons
	SupportedAddons = []string{"prometheus"}
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
	// createAddonsCmd.Flags().StringVarP(&cnpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	// createAddonsCmd.MarkFlagRequired("subscriptionid")
	// createAddonsCmd.Flags().StringVarP(&cnpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	// createAddonsCmd.MarkFlagRequired("resourcegroup")

	addonsCmd.AddCommand(createAddonsCmd)

	// Delete
	// deleteAddonsCmd.Flags().StringVarP(&dnpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	// deleteAddonsCmd.MarkFlagRequired("subscriptionid")
	// deleteAddonsCmd.Flags().StringVarP(&dnpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	// deleteAddonsCmd.MarkFlagRequired("resourcegroup")

	addonsCmd.AddCommand(deleteAddonsCmd)

	// Upgrade
	// updateAddonsCmd.Flags().StringVarP(&dnpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	// updateAddonsCmd.MarkFlagRequired("subscriptionid")
	// updateAddonsCmd.Flags().StringVarP(&dnpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	// updateAddonsCmd.MarkFlagRequired("resourcegroup")

	addonsCmd.AddCommand(updateAddonsCmd)

	for _, addon := range SupportedAddons {
		createAddonsCmd.AddCommand(
			&cobra.Command{
				Use:   addon,
				Short: "Create " + addon + " Addon",
				Long:  `Create ` + addon + ` Addon with one command`,
				Run: func(cmd *cobra.Command, args []string) {
					if err := RunCreateorUpdateAddon(addon); err != nil {
						log.Error(err, "Failed to create addon")
						os.Exit(1)
					}
				},
			})

		updateAddonsCmd.AddCommand(
			&cobra.Command{
				Use:   addon,
				Short: "Update " + addon + " Addon",
				Long:  `Update ` + addon + ` Addon with one command`,
				Run: func(cmd *cobra.Command, args []string) {
					if err := RunCreateorUpdateAddon(addon); err != nil {
						log.Error(err, "Failed to create addon")
						os.Exit(1)
					}
				},
			})
	}
}

func RunCreateorUpdateAddon(addon string) error {
	kubeconfigBytes, err := ioutil.ReadFile(os.Getenv("KUBECONFIG"))
	if err != nil {
		return err
	}
	return kubectlApplyFolder(addon, string(kubeconfigBytes), addons.Addons)
}
