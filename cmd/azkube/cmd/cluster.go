package cmd

import (
	"context"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"os"
	"strings"
	"time"

	enginev1alpha1 "github.com/awesomenix/azkube/pkg/apis/engine/v1alpha1"
	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Manage a Kubernetes Cluster on Azure",
	Long:  `Manage a Kubernetes Cluster on Azure with one command`,
}

func init() {
	RootCmd.AddCommand(clusterCmd)

	// Create
	createClusterCmd.Flags().StringVarP(&co.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	createClusterCmd.MarkFlagRequired("subscriptionid")
	createClusterCmd.Flags().StringVarP(&co.ClientID, "clientid", "c", "", "Client ID Required.")
	createClusterCmd.MarkFlagRequired("clientid")
	createClusterCmd.Flags().StringVarP(&co.ClientSecret, "clientsecret", "i", "", "Client Secret Required.")
	createClusterCmd.MarkFlagRequired("clientsecret")
	createClusterCmd.Flags().StringVarP(&co.TenantID, "tenantid", "t", "", "Tenant ID Required.")
	createClusterCmd.MarkFlagRequired("tenantid")
	createClusterCmd.Flags().StringVarP(&co.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	createClusterCmd.MarkFlagRequired("resourcegroup")
	createClusterCmd.Flags().StringVarP(&co.ResourceLocation, "location", "l", "", "Resource Group Location, in which all resources are created Required.")
	createClusterCmd.MarkFlagRequired("location")

	// Optional flags
	createClusterCmd.Flags().StringVarP(&co.MasterKubernetesVersion, "kubernetesversion", "k", "Stable", "Master Kubernetes version, Optional, Uses Stable version as default.")
	createClusterCmd.Flags().StringVarP(&co.KubeconfigOutput, "kubeconfigout", "o", "kubeconfig", "Where to output the kubeconfig for the provisioned cluster")

	clusterCmd.AddCommand(createClusterCmd)

	// Delete
	deleteClusterCmd.Flags().StringVarP(&do.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	deleteClusterCmd.MarkFlagRequired("subscriptionid")
	deleteClusterCmd.Flags().StringVarP(&do.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	deleteClusterCmd.MarkFlagRequired("resourcegroup")

	clusterCmd.AddCommand(deleteClusterCmd)

	// Upgrade
}

var createClusterCmd = &cobra.Command{
	Use:   "create",
	Short: "Create kubernetes cluster",
	Long:  `Create a kubernetes cluster with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := RunCreate(co); err != nil {
			log.Error(err, "Failed to create cluster")
			os.Exit(1)
		}
	},
}

var deleteClusterCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete kubernetes cluster",
	Long:  `Delete a kubernetes cluster with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := RunDelete(do); err != nil {
			log.Error(err, "Failed to delete cluster")
			os.Exit(1)
		}
	},
}

type CreateOptions struct {
	SubscriptionID          string
	ClientID                string
	ClientSecret            string
	TenantID                string
	ResourceGroup           string
	ResourceLocation        string
	MasterKubernetesVersion string
	KubeconfigOutput        string
}

type DeleteOptions struct {
	SubscriptionID string
	ResourceGroup  string
}

var co = &CreateOptions{}
var do = &DeleteOptions{}

func RunCreate(co *CreateOptions) error {
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
	h.Write([]byte(fmt.Sprintf("%s/%s", co.SubscriptionID, co.ResourceGroup)))
	clusterName := fmt.Sprintf("%x", h.Sum64())

	s := spinner.New(spinner.CharSets[11], 200*time.Millisecond)
	s.Color("green")
	s.Suffix = fmt.Sprintf(" Creating Namespace %s", clusterName)
	s.Start()
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: clusterName,
		},
	}

	if err := kClient.Create(context.TODO(), namespace); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Fprintf(s.Writer, " ✗ Failed to Create Namespace %v\n", err)
			return err
		}
	}
	s.Stop()

	fmt.Fprintf(s.Writer, " ✓ Successfully Created Namespace %s\n", clusterName)

	cluster := &enginev1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: clusterName,
		},
		Spec: enginev1alpha1.ClusterSpec{
			SubscriptionID:    co.SubscriptionID,
			ClientID:          co.ClientID,
			ClientSecret:      co.ClientSecret,
			TenantID:          co.TenantID,
			ResourceGroupName: co.ResourceGroup,
			Location:          co.ResourceLocation,
		},
	}

	s = spinner.New(spinner.CharSets[11], 200*time.Millisecond)
	s.Color("green")
	s.Suffix = fmt.Sprintf(" Creating Cluster %s with group %s in %s .. timeout 1m0s", clusterName, co.ResourceGroup, co.ResourceLocation)
	s.Start()

	if err := kClient.Create(context.TODO(), cluster); err != nil {
		fmt.Fprintf(s.Writer, " ✗ Failed to Create Cluster %v\n", err)
		return err
	}

	start := time.Now()
	cluster = &enginev1alpha1.Cluster{}
	for i := 0; i < 12; i++ {
		if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: clusterName}, cluster); err == nil {
			if cluster.Status.ProvisioningState == "Succeeded" {
				break
			}
		}
		time.Sleep(5 * time.Second)
	}
	s.Stop()

	if cluster.Status.ProvisioningState != "Succeeded" {
		fmt.Fprintf(s.Writer, " ✗ Failed to Create Cluster %v\n", err)
		return err
	}

	fmt.Fprintf(s.Writer, " ✓ Successfully Created Cluster %s in %s\n", clusterName, time.Since(start))

	if co.KubeconfigOutput != "" {
		ioutil.WriteFile(co.KubeconfigOutput, []byte(cluster.Status.CustomerKubeConfig), 0644)
	}

	controlPlane := &enginev1alpha1.ControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: clusterName,
		},
		Spec: enginev1alpha1.ControlPlaneSpec{
			KubernetesVersion: co.MasterKubernetesVersion,
		},
	}

	s = spinner.New(spinner.CharSets[11], 200*time.Millisecond)
	s.Color("green")
	s.Suffix = fmt.Sprintf(" Creating ControlPlane %s with kubernetes version %s .. timeout 15m0s", clusterName, co.MasterKubernetesVersion)
	s.Start()

	if err := kClient.Create(context.TODO(), controlPlane); err != nil {
		fmt.Fprintf(s.Writer, " ✗ Failed to Create ControlPlane %v\n", err)
		return err
	}

	start = time.Now()
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

	fmt.Fprintf(s.Writer, " ✓ Successfully Created ControlPlane with Kubernetes Version %s in %s\n", co.MasterKubernetesVersion, time.Since(start))

	return nil
}

func RunDelete(do *DeleteOptions) error {
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
	h.Write([]byte(fmt.Sprintf("%s/%s", do.SubscriptionID, do.ResourceGroup)))
	clusterName := fmt.Sprintf("%x", h.Sum64())

	log.Info("getting namespace", "Name", clusterName)

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: clusterName,
		},
	}

	log.Info("deleting namespace", "Name", clusterName)

	if err := kClient.Delete(context.TODO(), namespace); err != nil {
		log.Error(err, "failed to delete cluster", "Name", clusterName)
	}
	log.Info("successfully deleted namespace", "Name", clusterName)

	return nil
}
