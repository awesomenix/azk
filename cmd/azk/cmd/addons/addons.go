package addons

import (
	"io/ioutil"
	"os"

	addonassets "github.com/awesomenix/azk/addonassets"
	cmdhelpers "github.com/awesomenix/azk/cmd/azk/cmd/helpers"
	"github.com/spf13/cobra"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("azk")
	// SupportedAddons list of supported addons
	SupportedAddons = []string{"prometheus"}
)

var CreateAddonsCmd = &cobra.Command{
	Use:   "addons",
	Short: "Create Addons",
	Long:  `Create Addons with one command`,
}

var DeleteAddonsCmd = &cobra.Command{
	Use:   "addons",
	Short: "Create Addons",
	Long:  `Create Addons with one command`,
}

var UpgradeAddonsCmd = &cobra.Command{
	Use:   "addons",
	Short: "Upgrade Addons",
	Long:  `Upgrade Addons with one command`,
}

func init() {
	// Create
	// CreateAddonsCmd.Flags().StringVarP(&cnpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	// CreateAddonsCmd.MarkFlagRequired("subscriptionid")
	// CreateAddonsCmd.Flags().StringVarP(&cnpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	// CreateAddonsCmd.MarkFlagRequired("resourcegroup")

	// Delete
	// DeleteAddonsCmd.Flags().StringVarP(&dnpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	// DeleteAddonsCmd.MarkFlagRequired("subscriptionid")
	// DeleteAddonsCmd.Flags().StringVarP(&dnpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	// DeleteAddonsCmd.MarkFlagRequired("resourcegroup")

	// Upgrade
	// updateAddonsCmd.Flags().StringVarP(&dnpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	// updateAddonsCmd.MarkFlagRequired("subscriptionid")
	// updateAddonsCmd.Flags().StringVarP(&dnpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	// updateAddonsCmd.MarkFlagRequired("resourcegroup")

	for _, addon := range SupportedAddons {
		CreateAddonsCmd.AddCommand(
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

		UpgradeAddonsCmd.AddCommand(
			&cobra.Command{
				Use:   addon,
				Short: "Upgrade " + addon + " Addon",
				Long:  `Upgrade ` + addon + ` Addon with one command`,
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
	return cmdhelpers.KubectlApplyFolder(addon, string(kubeconfigBytes), addonassets.Addons)
}
