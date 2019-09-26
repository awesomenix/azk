package controlplane

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"time"

	enginev1alpha1 "github.com/awesomenix/azk/api/v1alpha1"
	"github.com/awesomenix/azk/helpers"
	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("azk")

func init() {
	// Create
	CreateControlPlaneCmd.Flags().StringVarP(&ccpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	CreateControlPlaneCmd.MarkFlagRequired("subscriptionid")
	CreateControlPlaneCmd.Flags().StringVarP(&ccpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	CreateControlPlaneCmd.MarkFlagRequired("resourcegroup")

	// Optional flags
	CreateControlPlaneCmd.Flags().StringVarP(&ccpo.MasterKubernetesVersion, "kubernetesversion", "k", "stable", "Master Kubernetes version, Optional, Uses Stable version as default.")

	// Upgrade
	UpgradeControlPlaneCmd.Flags().StringVarP(&ucpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	UpgradeControlPlaneCmd.MarkFlagRequired("subscriptionid")
	UpgradeControlPlaneCmd.Flags().StringVarP(&ucpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	UpgradeControlPlaneCmd.MarkFlagRequired("resourcegroup")

	// Optional flags
	UpgradeControlPlaneCmd.Flags().StringVarP(&ucpo.MasterKubernetesVersion, "kubernetesversion", "k", "stable", "Master Kubernetes version, Optional, Uses Stable version as default.")
}

var CreateControlPlaneCmd = &cobra.Command{
	Use:   "controlplane",
	Short: "Create kubernetes control plane",
	Long:  `Create a kubernetes control plane with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := CreateControlPlane(ccpo); err != nil {
			log.Error(err, "Failed to create cluster")
			os.Exit(1)
		}
	},
}

var UpgradeControlPlaneCmd = &cobra.Command{
	Use:   "controlplane",
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

type UpgradeControlPlaneOptions struct {
	SubscriptionID          string
	ResourceGroup           string
	MasterKubernetesVersion string
}

var ccpo = &CreateControlPlaneOptions{}
var ucpo = &UpgradeControlPlaneOptions{}

func CreateControlPlane(ccpo *CreateControlPlaneOptions) error {
	kubernetesVersion, err := helpers.GetKubernetesVersion(ccpo.MasterKubernetesVersion)
	if err != nil {
		log.Error(err, "Failed to determine valid kubernetes version")
		return err
	}
	ccpo.MasterKubernetesVersion = kubernetesVersion

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

	if controlPlane.Status.ProvisioningState != "Succeeded" {
		fmt.Fprintf(s.Writer, " ✗ Failed to Create Control Plane timedout\n")
		return err
	}

	fmt.Fprintf(s.Writer, " ✓ Successfully Created ControlPlane with Kubernetes Version %s in %s\n", ccpo.MasterKubernetesVersion, time.Since(start))

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

	cp := &enginev1alpha1.ControlPlane{}
	if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: clusterName}, cp); err != nil {
		log.Error(err, "Failed to get control plane", "Name", clusterName)
		return err
	}

	s := spinner.New(spinner.CharSets[11], 200*time.Millisecond)
	s.Color("green")
	s.Suffix = fmt.Sprintf(" Upgrading ControlPlane %s from %s to %s .. timeout 15m0s", cp.Name, cp.Status.KubernetesVersion, ucpo.MasterKubernetesVersion)
	s.Start()

	cp.Spec.KubernetesVersion = ucpo.MasterKubernetesVersion
	if err := kClient.Update(context.TODO(), cp); err != nil {
		log.Error(err, "Failed to upgrade control plane", "Name", clusterName)
		return err
	}

	start := time.Now()
	cp = &enginev1alpha1.ControlPlane{}
	for i := 0; i < 30; i++ {
		if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: clusterName}, cp); err == nil {
			if cp.Status.ProvisioningState == "Succeeded" &&
				cp.Status.KubernetesVersion == ucpo.MasterKubernetesVersion {
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
