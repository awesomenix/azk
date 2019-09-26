package controllers

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-03-01/compute"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	enginev1alpha1 "github.com/awesomenix/azk/api/v1alpha1"
	azhelpers "github.com/awesomenix/azk/azure"
	"github.com/awesomenix/azk/bootstrap"
	"github.com/awesomenix/azk/helpers"
)

const (
	nodesetsFinalizerName = "nodesets.finalizers.engine.azk.io"
)

func kubeadmNodeJoinConfig(internalDNSName, bootstrapToken, discoveryHash string) string {
	return fmt.Sprintf(`
cat <<EOF >/tmp/kubeadm-config.yaml
apiVersion: kubeadm.k8s.io/v1beta1
kind: JoinConfiguration
nodeRegistration:
  kubeletExtraArgs:
    cloud-provider: azure
    cloud-config: /etc/kubernetes/azure.json
discovery:
  bootstrapToken:
    token: %[1]s
    apiServerEndpoint: "%[2]s:6443"
    caCertHashes:
    - %[3]s
EOF
`, bootstrapToken,
		internalDNSName,
		discoveryHash,
	)
}

func getNodeSetStartupScript(kubernetesVersion, internalDNSName, bootstrapToken, discoveryHash string) string {
	return fmt.Sprintf(`
%[1]s
sudo cp -f /etc/hosts /tmp/hostsupdate
sudo chown $(id -u):$(id -g) /tmp/hostsupdate
echo '10.0.0.100 %[2]s' >> /tmp/hostsupdate
sudo mv /etc/hosts /etc/hosts.bak
sudo mv /tmp/hostsupdate /etc/hosts
%[3]s
#Setup using kubeadm
sudo kubeadm join --config /tmp/kubeadm-config.yaml
`, helpers.PreRequisitesInstallScript(kubernetesVersion),
		internalDNSName,
		kubeadmNodeJoinConfig(internalDNSName, bootstrapToken, discoveryHash),
	)
}

// NodeSetReconciler reconciles a NodeSet object
type NodeSetReconciler struct {
	client.Client
	Log logr.Logger
	record.EventRecorder
}

// +kubebuilder:rbac:groups=engine.azk.io,resources=nodesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=engine.azk.io,resources=nodesets/status,verbs=get;update;patch

func (r *NodeSetReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()

	defer helpers.Recover()
	// Fetch the NodeSet instance
	instance := &enginev1alpha1.NodeSet{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	cluster, err := r.getCluster(ctx, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	if cluster.Status.ProvisioningState != "Succeeded" {
		// Wait for cluster to initialize
		return ctrl.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
	}

	cloudConfig := azhelpers.CloudConfiguration{
		CloudName:      azhelpers.AzurePublicCloudName,
		SubscriptionID: cluster.Spec.SubscriptionID,
		ClientID:       cluster.Spec.ClientID,
		ClientSecret:   cluster.Spec.ClientSecret,
		TenantID:       cluster.Spec.TenantID,
		GroupName:      cluster.Spec.GroupName,
		GroupLocation:  cluster.Spec.GroupLocation,
		UserAgent:      "azk",
	}

	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object.
		if !helpers.ContainsFinalizer(instance.ObjectMeta.Finalizers, nodesetsFinalizerName) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, nodesetsFinalizerName)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{Requeue: true}, err
			}
			// Once updates object changes we need to requeue
			return ctrl.Result{Requeue: true}, nil
		}
	} else {
		if helpers.ContainsFinalizer(instance.ObjectMeta.Finalizers, nodesetsFinalizerName) {
			if cloudConfig.IsValid() {
				// our finalizer is present, so lets handle our external dependency
				if err := r.deleteNodeSet(ctx, instance, cloudConfig); err != nil {
					// if fail to delete the external dependency here, return with error
					// so that it can be retried
					// meh! its fine if it fails, we definitely need to wait here for it to be deleted
				}
			}

			// remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = helpers.RemoveFinalizer(instance.ObjectMeta.Finalizers, nodesetsFinalizerName)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{Requeue: true}, nil
			}
		}

		return ctrl.Result{}, nil
	}

	vmSKUType := instance.Spec.VMSKUType
	if vmSKUType == "" {
		vmSKUType = "Standard_DS2_v2"
	}

	if err := r.updateNodeSet(instance, cloudConfig); err != nil {
		instance.Status.ProvisioningState = "Updating"
		if err := r.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}

		customDataStr, err := getCustomData(instance, cluster)
		if err != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
		}

		subnetID := fmt.Sprintf(
			"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/azk-vnet/subnets/agent-subnet",
			cluster.Spec.SubscriptionID,
			cluster.Spec.GroupName)

		if err := cloudConfig.CreateVMSS(
			ctx,
			instance.Name+"-agentvmss",
			subnetID,
			nil,
			nil,
			customDataStr,
			vmSKUType,
			int(*instance.Spec.Replicas),
		); err != nil {
			return ctrl.Result{}, err
		}
		r.EventRecorder.Event(instance, "Normal", "Created", fmt.Sprintf("%s", instance.Name+"-agentvmss"))
		return ctrl.Result{Requeue: true}, nil
	} else if int(*instance.Spec.Replicas) != len(instance.Status.NodeStatus) {
		instance.Status.ProvisioningState = "Scaling"
		if err := r.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.scaleNodeSet(ctx, instance, cluster); err != nil {
			return ctrl.Result{}, err
		}
		r.EventRecorder.Event(instance, "Normal", "Scaled", fmt.Sprintf("%d to %d", len(instance.Status.NodeStatus), *instance.Spec.Replicas))
		return ctrl.Result{Requeue: true}, nil
	}

	if err := helpers.WaitForNodesReady(r.Client, instance.Name, int(*instance.Spec.Replicas)); err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
	}

	instance.Status.KubernetesVersion = instance.Spec.KubernetesVersion
	instance.Status.ProvisioningState = "Succeeded"
	instance.Status.Kubeconfig = cluster.Spec.CustomerKubeConfig
	instance.Status.Replicas = int32(len(instance.Status.NodeStatus))
	if err := r.Status().Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NodeSetReconciler) getCluster(ctx context.Context, namespace string) (*enginev1alpha1.Cluster, error) {
	clusterList := enginev1alpha1.ClusterList{}
	if err := r.Client.List(ctx, &clusterList, client.InNamespace(namespace)); err != nil {
		return nil, err
	}

	switch len(clusterList.Items) {
	case 0:
		return nil, fmt.Errorf("no clusters defined")
	case 1:
		return &clusterList.Items[0], nil
	default:
		return nil, fmt.Errorf("multiple clusters defined")
	}
}

func (r *NodeSetReconciler) deleteNodeSet(ctx context.Context, instance *enginev1alpha1.NodeSet, cloudConfig azhelpers.CloudConfiguration) error {
	log := r.Log.WithValues("nodeset", instance.Name)
	vmssName := instance.Name + "-agentvmss"

	for _, vms := range instance.Status.NodeStatus {
		err := helpers.CordonDrainAndDeleteNode(instance.Status.Kubeconfig, vms.VMComputerName)
		if err != nil {
			log.Info("Error in Cordon and Drain", "Error", err, "VM", vms.VMComputerName)
		}
	}
	log.Info("Deleting NodeSet", "VMSS", vmssName)
	if err := cloudConfig.DeleteVMSS(ctx, vmssName); err != nil {
		return err
	}
	return nil
}

func (r *NodeSetReconciler) updateNodeSet(instance *enginev1alpha1.NodeSet, cloudConfig azhelpers.CloudConfiguration) error {
	ctx := context.Background()
	log := r.Log.WithValues("nodeset", instance.Name)
	vmssName := instance.Name + "-agentvmss"
	vmssClient, err := cloudConfig.GetVMSSVMsClient()
	if err != nil {
		log.Error(err, "Error GetVMSSVMsClient", "VMSS", vmssName)
		return err
	}

	result, err := vmssClient.List(ctx, cloudConfig.GroupName, vmssName, "", "", string(compute.InstanceView))
	if err != nil {
		log.Error(err, "Error VMSSClient List", "VMSS", vmssName)
		return err
	}

	var vmStatus []enginev1alpha1.VMStatus
	for _, vmID := range result.Values() {
		log.Info("Appending to VMSS Nodepool list", "VM", *vmID.OsProfile.ComputerName)
		var status enginev1alpha1.VMStatus
		status.VMComputerName = *vmID.OsProfile.ComputerName
		status.VMInstanceID = *vmID.InstanceID
		vmStatus = append(vmStatus, status)
	}
	instance.Status.NodeStatus = vmStatus
	return nil
}

func (r *NodeSetReconciler) scaleNodeSet(ctx context.Context, instance *enginev1alpha1.NodeSet, cluster *enginev1alpha1.Cluster) error {
	log := r.Log.WithValues("nodeset", instance.Name)
	vmssName := instance.Name + "-agentvmss"
	expectedCount := int(*instance.Spec.Replicas)
	curCount := 0
	for _, nodeStatus := range instance.Status.NodeStatus {
		if curCount < expectedCount {
			curCount++
			continue
		}

		err := helpers.CordonDrainAndDeleteNode(instance.Status.Kubeconfig, nodeStatus.VMComputerName)
		if err != nil {
			return err
		}

		vmssClient, err := cluster.Spec.CloudConfiguration.GetVMSSVMsClient()
		if err != nil {
			return err
		}

		log.Info("Scaling down", "VMSS", nodeStatus.VMInstanceID)

		_, err = vmssClient.Delete(ctx, cluster.Spec.CloudConfiguration.GroupName, vmssName, nodeStatus.VMInstanceID)
		if err != nil {
			return err
		}
	}
	customDataStr, err := getCustomData(instance, cluster)
	if err != nil {
		return err
	}
	return cluster.Spec.CloudConfiguration.ScaleVMSS(ctx, vmssName, customDataStr, expectedCount)
}

func getCustomData(instance *enginev1alpha1.NodeSet, cluster *enginev1alpha1.Cluster) (string, error) {
	bootstrapToken, err := bootstrap.CreateNewBootstrapToken()
	if err != nil {
		return "", err
	}

	startupScript := getNodeSetStartupScript(
		instance.Spec.KubernetesVersion,
		cluster.Spec.InternalDNSName,
		bootstrapToken,
		cluster.Spec.DiscoveryHashes[0])

	customData := map[string]string{
		"/etc/kubernetes/azure.json": cluster.Spec.AzureCloudProviderConfig,
	}
	customRunData := map[string]string{
		"/etc/kubernetes/init-azure-bootstrap.sh": startupScript,
	}

	return base64.StdEncoding.EncodeToString([]byte(azhelpers.GetCustomData(customData, customRunData))), nil
}

func (r *NodeSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&enginev1alpha1.NodeSet{}).
		Complete(r)
}
