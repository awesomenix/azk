package nodepool

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

var cnpo = &CreateNodePoolOptions{}
var dnpo = &DeleteNodePoolOptions{}
var snpo = &ScaleNodePoolOptions{}
var unpo = &UpgradeNodePoolOptions{}

var CreateNodepoolCmd = &cobra.Command{
	Use:   "nodepool",
	Short: "Create Node Pool",
	Long:  `Create a Node Pool with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := CreateNodePool(cnpo); err != nil {
			log.Error(err, "Failed to create cluster")
			os.Exit(1)
		}
	},
}

var DeleteNodepoolCmd = &cobra.Command{
	Use:   "nodepool",
	Short: "Delete Node Pool",
	Long:  `Delete a node pool with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := DeleteNodePool(dnpo); err != nil {
			log.Error(err, "Failed to delete cluster")
			os.Exit(1)
		}
	},
}

var ScaleNodepoolCmd = &cobra.Command{
	Use:   "nodepool",
	Short: "Scale Node Pool",
	Long:  `Scale a node pool with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := ScaleNodePool(snpo); err != nil {
			log.Error(err, "Failed to scale cluster")
			os.Exit(1)
		}
	},
}

var UpgradeNodepoolCmd = &cobra.Command{
	Use:   "nodepool",
	Short: "Upgrade Node Pool",
	Long:  `Upgrade a node pool with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := UpgradeNodePool(unpo); err != nil {
			log.Error(err, "Failed to upgrade cluster")
			os.Exit(1)
		}
	},
}

func init() {
	// Create
	CreateNodepoolCmd.Flags().StringVarP(&cnpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	CreateNodepoolCmd.MarkFlagRequired("subscriptionid")
	CreateNodepoolCmd.Flags().StringVarP(&cnpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	CreateNodepoolCmd.MarkFlagRequired("resourcegroup")
	CreateNodepoolCmd.Flags().StringVarP(&cnpo.Name, "name", "n", "", "Nodepool Name Required.")
	CreateNodepoolCmd.MarkFlagRequired("name")
	CreateNodepoolCmd.Flags().Int32VarP(&cnpo.Count, "count", "c", 1, "Nodepool Count, Optional, default 1")

	// Optional flags
	CreateNodepoolCmd.Flags().StringVarP(&cnpo.AgentKubernetesVersion, "kubernetesversion", "k", "stable", "Agent Kubernetes version, Optional, Uses stable version as default.")

	// Delete
	DeleteNodepoolCmd.Flags().StringVarP(&dnpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	DeleteNodepoolCmd.MarkFlagRequired("subscriptionid")
	DeleteNodepoolCmd.Flags().StringVarP(&dnpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	DeleteNodepoolCmd.MarkFlagRequired("resourcegroup")
	DeleteNodepoolCmd.Flags().StringVarP(&dnpo.Name, "name", "n", "", "Nodepool Name Required.")
	DeleteNodepoolCmd.MarkFlagRequired("name")

	// Scale

	ScaleNodepoolCmd.Flags().StringVarP(&snpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	ScaleNodepoolCmd.MarkFlagRequired("subscriptionid")
	ScaleNodepoolCmd.Flags().StringVarP(&snpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	ScaleNodepoolCmd.MarkFlagRequired("resourcegroup")
	ScaleNodepoolCmd.Flags().StringVarP(&snpo.Name, "name", "n", "", "Nodepool Name Required.")
	ScaleNodepoolCmd.MarkFlagRequired("name")
	ScaleNodepoolCmd.Flags().Int32VarP(&snpo.Count, "count", "c", 1, "Nodepool Count, Optional, default 1")
	ScaleNodepoolCmd.MarkFlagRequired("count")

	// Upgrade
	UpgradeNodepoolCmd.Flags().StringVarP(&unpo.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	UpgradeNodepoolCmd.MarkFlagRequired("subscriptionid")
	UpgradeNodepoolCmd.Flags().StringVarP(&unpo.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	UpgradeNodepoolCmd.MarkFlagRequired("resourcegroup")
	UpgradeNodepoolCmd.Flags().StringVarP(&unpo.Name, "name", "n", "", "Nodepool Name Required.")
	UpgradeNodepoolCmd.MarkFlagRequired("name")
	UpgradeNodepoolCmd.Flags().StringVarP(&unpo.AgentKubernetesVersion, "kubernetesversion", "k", "stable", "Nodepool Kubernetes Version, Default. stable")
	UpgradeNodepoolCmd.MarkFlagRequired("kubernetesversion")
}

type CreateNodePoolOptions struct {
	SubscriptionID         string
	Name                   string
	ResourceGroup          string
	Count                  int32
	AgentKubernetesVersion string
}

type DeleteNodePoolOptions struct {
	SubscriptionID string
	ResourceGroup  string
	Name           string
}

type ScaleNodePoolOptions struct {
	SubscriptionID string
	ResourceGroup  string
	Name           string
	Count          int32
}

type UpgradeNodePoolOptions struct {
	SubscriptionID         string
	ResourceGroup          string
	Name                   string
	AgentKubernetesVersion string
}

func CreateNodePool(cnpo *CreateNodePoolOptions) error {
	kubernetesVersion, err := helpers.GetKubernetesVersion(cnpo.AgentKubernetesVersion)
	if err != nil {
		log.Error(err, "Failed to determine valid kubernetes version")
		return err
	}
	cnpo.AgentKubernetesVersion = kubernetesVersion

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
	h.Write([]byte(fmt.Sprintf("%s/%s", cnpo.SubscriptionID, cnpo.ResourceGroup)))
	clusterName := fmt.Sprintf("%x", h.Sum64())

	nodePool := &enginev1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cnpo.Name,
			Namespace: clusterName,
		},
		Spec: enginev1alpha1.NodePoolSpec{
			NodeSetSpec: enginev1alpha1.NodeSetSpec{
				KubernetesVersion: cnpo.AgentKubernetesVersion,
				Replicas:          &(cnpo.Count),
			},
		},
	}

	s := spinner.New(spinner.CharSets[11], 200*time.Millisecond)
	s.Color("green")
	s.Suffix = fmt.Sprintf(" Creating Nodepool %s with kubernetes version %s .. timeout 10m0s", cnpo.Name, cnpo.AgentKubernetesVersion)
	s.Start()

	if err := kClient.Create(context.TODO(), nodePool); err != nil {
		fmt.Fprintf(s.Writer, " ✗ Failed to Create Nodepool %v\n", err)
		return err
	}

	start := time.Now()
	nodePool = &enginev1alpha1.NodePool{}
	for i := 0; i < 20; i++ {
		if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: cnpo.Name}, nodePool); err == nil {
			if nodePool.Status.ProvisioningState == "Succeeded" {
				break
			}
		}
		time.Sleep(30 * time.Second)
	}
	s.Stop()

	if nodePool.Status.ProvisioningState != "Succeeded" {
		fmt.Fprintf(s.Writer, " ✗ Failed to Create NodePool %v\n", err)
		return err
	}

	fmt.Fprintf(s.Writer, " ✓ Successfully Created NodePool %s with Kubernetes Version %s in %s\n", cnpo.Name, cnpo.AgentKubernetesVersion, time.Since(start))

	return nil
}

func DeleteNodePool(dnpo *DeleteNodePoolOptions) error {
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
	h.Write([]byte(fmt.Sprintf("%s/%s", dnpo.SubscriptionID, dnpo.ResourceGroup)))
	clusterName := fmt.Sprintf("%x", h.Sum64())

	s := spinner.New(spinner.CharSets[11], 200*time.Millisecond)
	s.Color("green")
	s.Suffix = fmt.Sprintf(" Deleting Nodepool %s", dnpo.Name)
	s.Start()

	nodePool := &enginev1alpha1.NodePool{}
	if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: dnpo.Name}, nodePool); err != nil {
		log.Error(err, "Failed to get nodepool", "Name", dnpo.Name)
		return err
	}

	if err := kClient.Delete(context.TODO(), nodePool); err != nil {
		log.Error(err, "Failed to delete nodepool", "Name", dnpo.Name)
		return err
	}

	s.Stop()

	fmt.Fprintf(s.Writer, " ✓ Successfully Deleted Nodepool %s\n", dnpo.Name)

	return nil
}

func ScaleNodePool(snpo *ScaleNodePoolOptions) error {
	log.Info("setting up client for scale")
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
	h.Write([]byte(fmt.Sprintf("%s/%s", snpo.SubscriptionID, snpo.ResourceGroup)))
	clusterName := fmt.Sprintf("%x", h.Sum64())

	nodePool := &enginev1alpha1.NodePool{}
	if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: snpo.Name}, nodePool); err != nil {
		log.Error(err, "Failed to get nodepool", "Name", snpo.Name)
		return err
	}

	s := spinner.New(spinner.CharSets[11], 200*time.Millisecond)
	s.Color("green")
	s.Suffix = fmt.Sprintf(" Scaling Nodepool %s from %d to %d .. timeout 10m0s", snpo.Name, nodePool.Status.VMReplicas, snpo.Count)
	s.Start()

	nodePool.Spec.Replicas = &snpo.Count
	if err := kClient.Update(context.TODO(), nodePool); err != nil {
		log.Error(err, "Failed to scale nodepool", "Name", snpo.Name)
		return err
	}

	start := time.Now()
	nodePool = &enginev1alpha1.NodePool{}
	for i := 0; i < 20; i++ {
		if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: snpo.Name}, nodePool); err == nil {
			if nodePool.Status.ProvisioningState == "Succeeded" &&
				nodePool.Status.VMReplicas == snpo.Count {
				s.Stop()
				fmt.Fprintf(s.Writer, " ✓ Successfully Scaled Nodepool %s in %s\n", snpo.Name, time.Since(start))
				return nil
			}
		}
		time.Sleep(30 * time.Second)
	}
	s.Stop()

	fmt.Fprintf(s.Writer, " ✗ Failed to Scale Nodepool %s, timedout\n", snpo.Name)

	return nil
}

func UpgradeNodePool(unpo *UpgradeNodePoolOptions) error {
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
	h.Write([]byte(fmt.Sprintf("%s/%s", unpo.SubscriptionID, unpo.ResourceGroup)))
	clusterName := fmt.Sprintf("%x", h.Sum64())

	nodePool := &enginev1alpha1.NodePool{}
	if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: unpo.Name}, nodePool); err != nil {
		log.Error(err, "Failed to get nodepool", "Name", unpo.Name)
		return err
	}

	s := spinner.New(spinner.CharSets[11], 200*time.Millisecond)
	s.Color("green")
	s.Suffix = fmt.Sprintf(" Upgrading Nodepool %s from %s to %s .. timeout 15m0s", unpo.Name, nodePool.Status.KubernetesVersion, unpo.AgentKubernetesVersion)
	s.Start()

	nodePool.Spec.KubernetesVersion = unpo.AgentKubernetesVersion
	if err := kClient.Update(context.TODO(), nodePool); err != nil {
		log.Error(err, "Failed to upgrade nodepool", "Name", unpo.Name)
		return err
	}

	start := time.Now()
	nodePool = &enginev1alpha1.NodePool{}
	for i := 0; i < 30; i++ {
		if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: unpo.Name}, nodePool); err == nil {
			if nodePool.Status.ProvisioningState == "Succeeded" &&
				nodePool.Status.KubernetesVersion == unpo.AgentKubernetesVersion {
				s.Stop()
				fmt.Fprintf(s.Writer, " ✓ Successfully Upgraded Nodepool %s in %s\n", unpo.Name, time.Since(start))
				return nil
			}
		}
		time.Sleep(30 * time.Second)
	}
	s.Stop()

	fmt.Fprintf(s.Writer, " ✗ Failed to Upgrade Nodepool %s, timedout\n", unpo.Name)

	return nil
}
