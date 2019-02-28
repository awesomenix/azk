package cmd

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"time"

	enginev1alpha1 "github.com/awesomenix/azkube/pkg/apis/engine/v1alpha1"
	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var controlPlaneCmd = &cobra.Command{
	Use:   "controlplane",
	Short: "Manage a Control Plane for Kubernetes Cluster on Azure",
	Long:  `Manage a Control Plane for Kubernetes Cluster on Azure with one command`,
}

func init() {
	RootCmd.AddCommand(controlPlaneCmd)

	// Create
	createControlPlaneCmd.Flags().StringVarP(&ccpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	createControlPlaneCmd.MarkFlagRequired("subscriptionid")
	createControlPlaneCmd.Flags().StringVarP(&ccpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	createControlPlaneCmd.MarkFlagRequired("resourcegroup")

	// Optional flags
	createControlPlaneCmd.Flags().StringVarP(&ccpo.MasterKubernetesVersion, "kubernetesversion", "k", "Stable", "Master Kubernetes version, Optional, Uses Stable version as default.")

	controlPlaneCmd.AddCommand(createControlPlaneCmd)

	// Delete
	deleteControlPlaneCmd.Flags().StringVarP(&dcpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	deleteControlPlaneCmd.MarkFlagRequired("subscriptionid")
	deleteControlPlaneCmd.Flags().StringVarP(&dcpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	deleteControlPlaneCmd.MarkFlagRequired("resourcegroup")

	controlPlaneCmd.AddCommand(deleteControlPlaneCmd)

	// Upgrade
	upgradeControlPlaneCmd.Flags().StringVarP(&ucpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	upgradeControlPlaneCmd.MarkFlagRequired("subscriptionid")
	upgradeControlPlaneCmd.Flags().StringVarP(&ucpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	upgradeControlPlaneCmd.MarkFlagRequired("resourcegroup")

	// Optional flags
	upgradeControlPlaneCmd.Flags().StringVarP(&ucpo.MasterKubernetesVersion, "kubernetesversion", "k", "Stable", "Master Kubernetes version, Optional, Uses Stable version as default.")
	controlPlaneCmd.AddCommand(upgradeControlPlaneCmd)
}

var createControlPlaneCmd = &cobra.Command{
	Use:   "create",
	Short: "Create kubernetes control plane",
	Long:  `Create a kubernetes control plane with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := CreateControlPlane(ccpo); err != nil {
			log.Error(err, "Failed to create cluster")
			os.Exit(1)
		}
	},
}

var deleteControlPlaneCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete kubernetes control plane",
	Long:  `Delete a kubernetes control plane with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := DeleteControlPlane(dcpo); err != nil {
			log.Error(err, "Failed to delete control plane")
			os.Exit(1)
		}
	},
}

var upgradeControlPlaneCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade kubernetes control plane",
	Long:  `Upgrade a kubernetes control plane with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := UpgradeControlPlane(ucpo); err != nil {
			log.Error(err, "Failed to upgrade control plane")
			os.Exit(1)
		}
	},
}

type CreateControlPlaneOptions struct {
	SubscriptionID          string
	ResourceGroup           string
	MasterKubernetesVersion string
}

type DeleteControlPlaneOptions struct {
	SubscriptionID string
	ResourceGroup  string
}

type UpgradeControlPlaneOptions struct {
	SubscriptionID          string
	ResourceGroup           string
	MasterKubernetesVersion string
}

var ccpo = &CreateControlPlaneOptions{}
var dcpo = &DeleteControlPlaneOptions{}
var ucpo = &UpgradeControlPlaneOptions{}

func CreateControlPlane(ccpo *CreateControlPlaneOptions) error {
	// Get a config to talk to the apiserver
	log.Info("setting up client for create")
	cfg, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		log.Error(err, "Failed to create config from KUBECONFIG")
		return err
	}

	kClient, err := client.New(cfg, client.Options{})
	if err != nil {
		log.Error(err, "Failed to create kube client from config")
		return err
	}

	h := fnv.New64a()
	h.Write([]byte(fmt.Sprintf("%s/%s", ccpo.SubscriptionID, ccpo.ResourceGroup)))
	clusterName := fmt.Sprintf("%x", h.Sum64())

	cluster := &enginev1alpha1.Cluster{}
	if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: clusterName}, cluster); err != nil {
		log.Error(err, "Failed to get cluster", "Name", clusterName)
		return err
	}

	controlPlane := &enginev1alpha1.ControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: clusterName,
		},
		Spec: enginev1alpha1.ControlPlaneSpec{
			KubernetesVersion: ccpo.MasterKubernetesVersion,
		},
	}

	s := spinner.New(spinner.CharSets[11], 200*time.Millisecond)
	s.Color("green")
	s.Suffix = fmt.Sprintf(" Creating ControlPlane %s with kubernetes version %s .. timeout 15m0s", clusterName, ccpo.MasterKubernetesVersion)
	s.Start()

	if err := kClient.Create(context.TODO(), controlPlane); err != nil {
		fmt.Fprintf(s.Writer, " ✗ Failed to Create ControlPlane %v\n", err)
		return err
	}

	start := time.Now()
	controlPlane = &enginev1alpha1.ControlPlane{}
	for i := 0; i < 30; i++ {
		if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: clusterName}, controlPlane); err == nil {
			if controlPlane.Status.ProvisioningState == "Succeeded" {
				break
			}
		}
		time.Sleep(30 * time.Second)
	}
	s.Stop()

	if cluster.Status.ProvisioningState != "Succeeded" {
		fmt.Fprintf(s.Writer, " ✗ Failed to Create Control Plane %v\n", err)
		return err
	}

	fmt.Fprintf(s.Writer, " ✓ Successfully Created ControlPlane with Kubernetes Version %s in %s\n", ccpo.MasterKubernetesVersion, time.Since(start))

	return nil
}

func DeleteControlPlane(dcpo *DeleteControlPlaneOptions) error {
	log.Info("setting up client for delete")
	cfg, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		log.Error(err, "Failed to create config from KUBECONFIG")
		return err
	}

	kClient, err := client.New(cfg, client.Options{})
	if err != nil {
		log.Error(err, "Failed to create kube client from config")
		return err
	}

	h := fnv.New64a()
	h.Write([]byte(fmt.Sprintf("%s/%s", dcpo.SubscriptionID, dcpo.ResourceGroup)))
	clusterName := fmt.Sprintf("%x", h.Sum64())

	log.Info("getting control plane", "Name", clusterName)

	controlPlane := &enginev1alpha1.ControlPlane{}

	if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: clusterName}, controlPlane); err == nil {
		log.Error(err, "failed to get control plane", "Name", clusterName)
	}

	log.Info("deleting control plane", "Name", clusterName)

	if err := kClient.Delete(context.TODO(), controlPlane); err != nil {
		log.Error(err, "failed to delete control plane", "Name", clusterName)
	}
	log.Info("successfully deleted control plane", "Name", clusterName)

	return nil
}

func UpgradeControlPlane(ucpo *UpgradeControlPlaneOptions) error {
	log.Info("setting up client for upgrade")
	cfg, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		log.Error(err, "Failed to create config from KUBECONFIG")
		return err
	}

	kClient, err := client.New(cfg, client.Options{})
	if err != nil {
		log.Error(err, "Failed to create kube client from config")
		return err
	}

	h := fnv.New64a()
	h.Write([]byte(fmt.Sprintf("%s/%s", ucpo.SubscriptionID, ucpo.ResourceGroup)))
	clusterName := fmt.Sprintf("%x", h.Sum64())

	controlplane := &enginev1alpha1.ControlPlane{}
	if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: clusterName}, controlplane); err != nil {
		log.Error(err, "Failed to get control plane", "Name", clusterName)
		return err
	}

	s := spinner.New(spinner.CharSets[11], 200*time.Millisecond)
	s.Color("green")
	s.Suffix = fmt.Sprintf(" Upgrading ControlPlane %s from %s to %s .. timeout 15m0s", unpo.Name, controlplane.Status.KubernetesVersion, ucpo.MasterKubernetesVersion)
	s.Start()

	controlplane.Spec.KubernetesVersion = ucpo.MasterKubernetesVersion
	if err := kClient.Update(context.TODO(), controlplane); err != nil {
		log.Error(err, "Failed to upgrade control plane", "Name", clusterName)
		return err
	}

	start := time.Now()
	controlplane = &enginev1alpha1.ControlPlane{}
	for i := 0; i < 30; i++ {
		if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: clusterName}, controlplane); err == nil {
			if controlplane.Status.ProvisioningState == "Succeeded" &&
				controlplane.Status.KubernetesVersion == ucpo.MasterKubernetesVersion {
				s.Stop()
				fmt.Fprintf(s.Writer, " ✓ Successfully Upgraded Control Plane %s in %s\n", clusterName, time.Since(start))
				return nil
			}
		}
		time.Sleep(30 * time.Second)
	}
	s.Stop()

	fmt.Fprintf(s.Writer, " ✗ Failed to Upgrade Control Plane %s, timedout\n", clusterName)

	return nil
}
