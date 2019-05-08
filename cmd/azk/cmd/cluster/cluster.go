package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/awesomenix/azk/assets"
	cmdhelpers "github.com/awesomenix/azk/cmd/azk/cmd/helpers"
	enginev1alpha1 "github.com/awesomenix/azk/pkg/apis/engine/v1alpha1"
	azhelpers "github.com/awesomenix/azk/pkg/azure"
	"github.com/awesomenix/azk/pkg/bootstrap"
	"github.com/awesomenix/azk/pkg/helpers"
	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("azk")

func init() {
	// Create
	CreateClusterCmd.Flags().StringVarP(&co.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	CreateClusterCmd.MarkFlagRequired("subscriptionid")
	CreateClusterCmd.Flags().StringVarP(&co.ClientID, "clientid", "i", "", "Client ID Required.")
	CreateClusterCmd.MarkFlagRequired("clientid")
	CreateClusterCmd.Flags().StringVarP(&co.ClientSecret, "clientsecret", "e", "", "Client Secret Required.")
	CreateClusterCmd.MarkFlagRequired("clientsecret")
	CreateClusterCmd.Flags().StringVarP(&co.TenantID, "tenantid", "t", "", "Tenant ID Required.")
	CreateClusterCmd.MarkFlagRequired("tenantid")
	CreateClusterCmd.Flags().StringVarP(&co.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	CreateClusterCmd.MarkFlagRequired("resourcegroup")
	CreateClusterCmd.Flags().StringVarP(&co.ResourceLocation, "location", "l", "", "Resource Group Location, in which all resources are created Required.")
	CreateClusterCmd.MarkFlagRequired("location")
	CreateClusterCmd.Flags().StringVarP(&co.DNSPrefix, "dnsprefix", "d", "dnsprefix", "DNS prefix for public loadbalancer")
	CreateClusterCmd.Flags().StringVarP(&co.KubernetesVersion, "kubernetesversion", "k", "stable", "Master Kubernetes Version")
	CreateClusterCmd.Flags().StringVarP(&co.NodePoolName, "nodepoolname", "n", "nodepool1", "Nodepool Name, Optional, default nodepool1")
	CreateClusterCmd.Flags().Int32VarP(&co.NodePoolCount, "nodepoolcount", "c", 1, "Nodepool Count, Optional, default 1")
	CreateClusterCmd.Flags().BoolVarP(&co.IsDevelopment, "isdev", "m", false, "Is development mode")
	CreateClusterCmd.Flags().StringVarP(&co.VMSKUType, "vmskutype", "u", "Standard_DS2_v2", "VM SKU Type, default: Standard_DS2_v2")

	// Optional flags
	CreateClusterCmd.Flags().StringVarP(&co.KubeconfigOutput, "kubeconfigout", "o", "kubeconfig", "Where to output the kubeconfig for the provisioned cluster")

	// Delete
	DeleteClusterCmd.Flags().StringVarP(&do.SubscriptionID, "subscriptionid", "s", "", "SubscriptionID Required.")
	DeleteClusterCmd.MarkFlagRequired("subscriptionid")
	DeleteClusterCmd.Flags().StringVarP(&do.ResourceGroup, "resourcegroup", "r", "", "Resource Group Name, in which all resources are created Required.")
	DeleteClusterCmd.MarkFlagRequired("resourcegroup")
}

var CreateClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Create kubernetes cluster",
	Long:  `Create a kubernetes cluster with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := RunCreate(co); err != nil {
			log.Error(err, "Failed to create cluster")
			os.Exit(1)
		}
	},
}

var DeleteClusterCmd = &cobra.Command{
	Use:   "cluster",
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
	SubscriptionID    string
	ClientID          string
	ClientSecret      string
	TenantID          string
	ResourceGroup     string
	ResourceLocation  string
	DNSPrefix         string
	KubernetesVersion string
	NodePoolName      string
	NodePoolCount     int32
	KubeconfigOutput  string
	VMSKUType         string
	IsDevelopment     bool
}

type DeleteOptions struct {
	SubscriptionID string
	ResourceGroup  string
}

var co = &CreateOptions{}
var do = &DeleteOptions{}

func RunCreate(co *CreateOptions) error {
	kubernetesVersion, err := helpers.GetKubernetesVersion(co.KubernetesVersion)
	if err != nil {
		log.Error(err, "Failed to determine valid kubernetes version")
		return err
	}
	co.KubernetesVersion = kubernetesVersion

	h := fnv.New64a()
	h.Write([]byte(fmt.Sprintf("%s/%s", co.SubscriptionID, co.ResourceGroup)))
	clusterName := fmt.Sprintf("%x", h.Sum64())

	clusterdir := os.Getenv("HOME") + "/.azk/" + clusterName
	if err := os.MkdirAll(clusterdir, 0755); err != nil {
		log.Error(err, "Failed to marshal bootstrap spec to json")
		return err
	}

	clusterStart := time.Now()
	log.Info("Creating Cluster", "KubernetesVersion", co.KubernetesVersion, "ClusterName", clusterName)

	spec, err := bootstrap.CreateSpec(&azhelpers.CloudConfiguration{
		CloudName:      azhelpers.AzurePublicCloudName,
		SubscriptionID: co.SubscriptionID,
		ClientID:       co.ClientID,
		ClientSecret:   co.ClientSecret,
		TenantID:       co.TenantID,
		GroupName:      co.ResourceGroup,
		GroupLocation:  co.ResourceLocation,
		UserAgent:      "azk",
	}, co.DNSPrefix, "", co.KubernetesVersion)

	if err != nil {
		log.Error(err, "Failed to create bootstrap spec")
		return err
	}
	spec.BootstrapVMSKUType = co.VMSKUType

	jsonSpec, err := json.Marshal(spec)
	if err != nil {
		log.Error(err, "Failed to marshal bootstrap spec to json")
		return err
	}

	if err := ioutil.WriteFile(clusterdir+"/bootstrapspec.json", jsonSpec, 0644); err != nil {
		log.Error(err, "Failed to store bootstrap spec")
		return err
	}

	s := spinner.New(spinner.CharSets[11], 200*time.Millisecond)
	s.Color("green")
	s.Suffix = fmt.Sprintf(" Creating bootstrap resources %s in group %s", spec.ClusterName, co.ResourceGroup)
	s.Start()
	start := time.Now()
	err = spec.Bootstrap()
	s.Stop()

	if err != nil {
		fmt.Fprintf(s.Writer, " ✗ Failed to create bootstrap resources %v\n", err)
		s = spinner.New(spinner.CharSets[11], 200*time.Millisecond)
		s.Color("green")
		s.Suffix = fmt.Sprintf(" Cleaning up bootstrap resources %s", spec.ClusterName)
		s.Start()
		spec.CleanupInfrastructure()
		s.Stop()
		fmt.Fprintf(s.Writer, " ✓ Successfully cleanup up bootstrap resources %s\n", spec.ClusterName)
		return err
	}

	fmt.Fprintf(s.Writer, " ✓ Successfully created bootstrap resources %s in %s\n", spec.ClusterName, time.Since(start))

	if co.KubeconfigOutput != "" {
		ioutil.WriteFile(co.KubeconfigOutput+"-"+clusterName, []byte(spec.CustomerKubeConfig), 0644)
	}

	// Get a config to talk to the apiserver

	clientcfg, err := clientcmd.NewClientConfigFromBytes([]byte(spec.CustomerKubeConfig))
	if err != nil {
		log.Error(err, "Failed to create config")
		return err
	}

	cfg, err := clientcfg.ClientConfig()
	if err != nil {
		log.Error(err, " ✗ Failed to get client config")
		return err
	}

	s = spinner.New(spinner.CharSets[11], 200*time.Millisecond)
	s.Color("green")
	s.Suffix = fmt.Sprintf(" Waiting for Stabilization ... 5m0s")
	s.Start()

	var loopErr error
	for i := 0; i < 100; i++ {
		waitclient, err := client.New(cfg, client.Options{})
		if err != nil {
			loopErr = err
			time.Sleep(3 * time.Second)
			continue
		}
		if err := helpers.WaitForNodesReady(waitclient, "azk-master-vmss", 1); err != nil {
			loopErr = err
			time.Sleep(3 * time.Second)
			continue
		}
		loopErr = nil
		break
	}
	s.Stop()

	if loopErr != nil {
		log.Error(loopErr, " ✗ Failed to create kube client from config")
		return loopErr
	}
	fmt.Fprintf(s.Writer, " ✓ Done\n")

	if err := kubectlApplyResources(spec.CustomerKubeConfig, co.IsDevelopment); err != nil {
		log.Error(err, "Failed to apply resources to bootstrap cluster")
		return err
	}

	s = spinner.New(spinner.CharSets[11], 200*time.Millisecond)
	s.Color("green")
	s.Suffix = fmt.Sprintf(" Creating Namespace %s", clusterName)
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: clusterName,
		},
	}

	kClient, err := client.New(cfg, client.Options{})
	if err != nil {
		log.Error(err, "Failed to create kube client from config")
		loopErr = err
	}

	s.Start()
	err = kClient.Create(context.TODO(), namespace)
	s.Stop()

	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Fprintf(s.Writer, " ✗ Failed to Create Namespace %v\n", err)
			return err
		}
	}

	fmt.Fprintf(s.Writer, " ✓ Successfully Created Namespace %s\n", clusterName)

	cluster := &enginev1alpha1.Cluster{}
	if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: clusterName}, cluster); err == nil {
		if cluster.Status.ProvisioningState == "Succeeded" {
			fmt.Fprintf(s.Writer, " ✓ Already Created Cluster %s\n", clusterName)
		}
		clusterSpec, err := yaml.Marshal(cluster)
		if err == nil {
			ioutil.WriteFile(clusterdir+"/clusterspec.yml", clusterSpec, 0644)
		}
	} else {
		time.Sleep(3 * time.Second)
		cluster = &enginev1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Namespace: clusterName,
			},
			Spec: enginev1alpha1.ClusterSpec{
				Spec: *spec,
			},
		}

		clusterSpec, err := yaml.Marshal(cluster)
		if err == nil {
			ioutil.WriteFile(clusterdir+"/clusterspec.yml", clusterSpec, 0644)
		}

		s = spinner.New(spinner.CharSets[11], 200*time.Millisecond)
		s.Color("green")
		s.Suffix = fmt.Sprintf(" Creating Cluster %s with group %s in %s", clusterName, co.ResourceGroup, co.ResourceLocation)

		s.Start()
		for i := 0; i < 10; i++ {
			err = kClient.Create(context.TODO(), cluster)
			if err != nil {
				time.Sleep(3 * time.Second)
				continue
			}
			break
		}
		s.Stop()

		if err != nil {
			fmt.Fprintf(s.Writer, " ✗ Failed to Create Cluster %v\n", err)
			return err
		}

		fmt.Fprintf(s.Writer, " ✓ Successfully Created Cluster %s\n", clusterName)
	}

	log.Info("Creating Control Plane and Node pool, using in-cluster operators")

	var cpError, npError error
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		controlPlane := &enginev1alpha1.ControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Namespace: clusterName,
			},
			Spec: enginev1alpha1.ControlPlaneSpec{
				KubernetesVersion: co.KubernetesVersion,
				VMSKUType:         co.VMSKUType,
			},
		}

		controlplaneSpec, err := yaml.Marshal(controlPlane)
		if err == nil {
			ioutil.WriteFile(clusterdir+"/controlplanespec.yml", controlplaneSpec, 0644)
		}

		log.Info("Creating ControlPlane .. timeout 15m0s", "ClusterName", clusterName, "KubernetesVersion", co.KubernetesVersion)

		if err := kClient.Create(context.TODO(), controlPlane); err != nil {
			log.Error(err, " ✗ Failed to Create ControlPlane")
			cpError = err
			return
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
			log.Error(err, " ✗ Failed to Create ControlPlane, timedout")
			cpError = err
			return
		}

		log.Info("✓ Successfully Created ControlPlane", "ClusterName", clusterName, "KubernetesVersion", co.KubernetesVersion, "TotalTime", time.Since(start))
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		nodePool := &enginev1alpha1.NodePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      co.NodePoolName,
				Namespace: clusterName,
			},
			Spec: enginev1alpha1.NodePoolSpec{
				NodeSetSpec: enginev1alpha1.NodeSetSpec{
					KubernetesVersion: co.KubernetesVersion,
					Replicas:          &(co.NodePoolCount),
					VMSKUType:         co.VMSKUType,
				},
			},
		}

		nodepoolSpec, err := yaml.Marshal(nodePool)
		if err == nil {
			ioutil.WriteFile(clusterdir+"/nodepoolspec.yml", nodepoolSpec, 0644)
		}

		log.Info("Creating Nodepool .. timeout 10m0s", "Name", co.NodePoolName, "KubernetesVersion", co.KubernetesVersion)

		if err := kClient.Create(context.TODO(), nodePool); err != nil {
			log.Error(err, " ✗ Failed to Create Nodepool")
			npError = err
			return
		}

		start := time.Now()
		nodePool = &enginev1alpha1.NodePool{}
		for i := 0; i < 20; i++ {
			if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: co.NodePoolName}, nodePool); err == nil {
				if nodePool.Status.ProvisioningState == "Succeeded" {
					break
				}
			}
			time.Sleep(30 * time.Second)
		}
		s.Stop()

		if nodePool.Status.ProvisioningState != "Succeeded" {
			log.Error(err, " ✗ Failed to Create NodePool, timedout", "Name", co.NodePoolName, "KubernetesVersion", co.KubernetesVersion)
			npError = err
			return
		}

		log.Info(" ✓ Successfully Created NodePool", "Name", co.NodePoolName, "KubernetesVersion", co.KubernetesVersion, "TotalTime", time.Since(start))
	}()

	wg.Wait()

	if cpError != nil {
		fmt.Fprintf(s.Writer, "\n ✗ Failed to Create Control Plane \n")
		return cpError
	}

	if npError != nil {
		fmt.Fprintf(s.Writer, "\n ✗ Failed to Create Node Pool \n")
		return npError
	}

	fmt.Fprintf(s.Writer, "\n ✓ Successfully Created Cluster %s in %s\n", clusterName, time.Since(clusterStart))

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

	cluster := &enginev1alpha1.Cluster{}
	if err := kClient.Get(context.TODO(), types.NamespacedName{Namespace: clusterName, Name: clusterName}, cluster); err != nil {
		log.Error(err, "Failed to get cluster")
		return err
	}

	s := spinner.New(spinner.CharSets[11], 200*time.Millisecond)
	s.Color("green")
	s.Suffix = fmt.Sprintf(" Deleting Cluster %s with group %s", clusterName, do.ResourceGroup)
	s.Start()

	start := time.Now()
	err = cluster.Spec.CleanupInfrastructure()
	s.Stop()

	if err != nil {
		fmt.Fprintf(s.Writer, " ✗ Failed to Delete Cluster %v\n", err)
		return err
	}

	fmt.Fprintf(s.Writer, " ✓ Successfully Deleted Cluster %s in %s\n", clusterName, time.Since(start))

	return nil
}

func kubectlApplyResources(kubeconfig string, isDevlopment bool) error {
	folders := []string{"crds", "deployment"}
	if isDevlopment {
		folders = []string{"crds"}
	}

	for _, folder := range folders {
		if err := cmdhelpers.KubectlApplyFolder(folder, kubeconfig, assets.Assets); err != nil {
			return err
		}
	}
	return nil
}
