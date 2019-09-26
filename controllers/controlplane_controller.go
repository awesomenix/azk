package controllers

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/Azure/go-autorest/autorest/to"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-03-01/compute"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	enginev1alpha1 "github.com/awesomenix/azk/api/v1alpha1"
	azhelpers "github.com/awesomenix/azk/azure"
	"github.com/awesomenix/azk/bootstrap"
	"github.com/awesomenix/azk/helpers"
)

const (
	masterVmssName = "azk-master-vmss"
)

func preRequisites(kubernetesVersion, apiServerIP, internalDNSName string) string {
	return fmt.Sprintf(`
%[1]s
sudo cp -f /etc/hosts /tmp/hostsupdate
sudo chown $(id -u):$(id -g) /tmp/hostsupdate
echo '%[2]s %[3]s' >> /tmp/hostsupdate
sudo mv /etc/hosts /etc/hosts.bak
sudo mv /tmp/hostsupdate /etc/hosts
`, helpers.PreRequisitesInstallScript(kubernetesVersion), apiServerIP, internalDNSName)
}

func kubeadmCPJoinConfig(bootstrapToken, internalDNSName, discoveryHash string) string {
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
controlPlane:
  localAPIEndpoint:
EOF
`, bootstrapToken,
		internalDNSName,
		discoveryHash,
	)
}

func getMasterStartupScript(kubernetesVersion, apiServerIP, internalDNSName, bootstrapToken, discoveryHash, etcdEndpoints string) string {
	return fmt.Sprintf(`
set -eux
%[1]s
%[2]s
#Setup using kubeadm
until sudo kubeadm join --config /tmp/kubeadm-config.yaml > /dev/null; do
	MEMBER_ID=$(sudo etcdctl --cert-file /etc/kubernetes/pki/etcd/server.crt --key-file /etc/kubernetes/pki/etcd/server.key --ca-file /etc/kubernetes/pki/etcd/ca.crt --endpoints \"%[4]s\" member list | grep -i $(uname -n) | cut -d ':' -f1)
	[ ! -z "$MEMBER_ID" ] && sudo etcdctl --cert-file /etc/kubernetes/pki/etcd/server.crt --key-file /etc/kubernetes/pki/etcd/server.key --ca-file /etc/kubernetes/pki/etcd/ca.crt --endpoints \"%[4]s\" member remove $MEMBER_ID
	sudo rm -rf /etc/kubernetes/manifests
	sleep 30
done
sudo cp -f /etc/hosts.bak /tmp/hostsupdate
sudo chown $(id -u):$(id -g) /tmp/hostsupdate
echo '127.0.0.1 %[3]s' >> /tmp/hostsupdate
sudo mv /tmp/hostsupdate /etc/hosts
`, preRequisites(kubernetesVersion, apiServerIP, internalDNSName),
		kubeadmCPJoinConfig(bootstrapToken, internalDNSName, discoveryHash),
		internalDNSName,
		etcdEndpoints,
	)
}

func getUpgradeScript(instance *enginev1alpha1.ControlPlane) string {
	return fmt.Sprintf(`
sudo apt-get upgrade -y kubectl=%[1]s-00 kubeadm=%[1]s-00
sudo kubeadm upgrade apply --force --yes v%[1]s
sudo kubectl --kubeconfig /etc/kubernetes/admin.conf drain $(uname -n) --ignore-daemonsets
sudo apt-mark unhold kubelet
sudo apt-get upgrade -y kubelet=%[1]s-00 
sudo apt-mark hold kubelet
sudo systemctl restart kubelet
sudo kubectl --kubeconfig /etc/kubernetes/admin.conf uncordon $(uname -n)
`, instance.Spec.KubernetesVersion)
}

// ControlPlaneReconciler reconciles a ControlPlane object
type ControlPlaneReconciler struct {
	client.Client
	Log logr.Logger
	record.EventRecorder
}

// +kubebuilder:rbac:groups=engine.azk.io,resources=controlplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=engine.azk.io,resources=controlplanes/status,verbs=get;update;patch

func (r *ControlPlaneReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("controlplane", req.NamespacedName)

	defer helpers.Recover()
	// Fetch the ControlPlane instance
	instance := &enginev1alpha1.ControlPlane{}
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

	if instance.Spec.KubernetesVersion == instance.Status.KubernetesVersion {
		return ctrl.Result{}, nil
	}

	cluster, err := r.getCluster(ctx, instance.Namespace)
	if err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
	}

	if cluster.Status.ProvisioningState != "Succeeded" {
		// Wait for cluster to initialize
		return ctrl.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
	}

	if err := r.updateVMSSStatus(instance, cluster); err != nil {
		return ctrl.Result{}, err
	}

	if instance.Status.ProvisioningState == "Succeeded" &&
		instance.Spec.KubernetesVersion == instance.Status.KubernetesVersion {
		return ctrl.Result{}, nil
	}

	log.Info("Updating Control Plane",
		"CurrentKubernetesVersion", instance.Status.KubernetesVersion,
		"ExpectedKubernetesVersion", instance.Spec.KubernetesVersion)

	instance.Status.ProvisioningState = "Updating"
	if err := r.Status().Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	customData := map[string]string{
		"/etc/kubernetes/pki/ca.crt":             cluster.Spec.CACertificate,
		"/etc/kubernetes/pki/ca.key":             cluster.Spec.CACertificateKey,
		"/etc/kubernetes/pki/sa.key":             cluster.Spec.ServiceAccountKey,
		"/etc/kubernetes/pki/sa.pub":             cluster.Spec.ServiceAccountPub,
		"/etc/kubernetes/pki/front-proxy-ca.crt": cluster.Spec.FrontProxyCACertificate,
		"/etc/kubernetes/pki/front-proxy-ca.key": cluster.Spec.FrontProxyCACertificateKey,
		"/etc/kubernetes/pki/etcd/ca.crt":        cluster.Spec.EtcdCACertificate,
		"/etc/kubernetes/pki/etcd/ca.key":        cluster.Spec.EtcdCACertificateKey,
		"/etc/kubernetes/azure.json":             cluster.Spec.AzureCloudProviderConfig,
		//"/etc/kubernetes/admin.conf":             cluster.Status.AdminKubeConfig,
	}

	vmSKUType := instance.Spec.VMSKUType
	if vmSKUType == "" {
		vmSKUType = "Standard_DS2_v2"
	}

	bootstrapToken, err := bootstrap.CreateNewBootstrapToken()
	if err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
	}

	var etcdEndpoints string
	for _, nodeStatus := range instance.Status.NodeStatus {
		if etcdEndpoints == "" {
			etcdEndpoints = fmt.Sprintf("https://%s:2379", nodeStatus.VMComputerName)
			continue
		}
		etcdEndpoints = fmt.Sprintf("%s,https://%s:2379", etcdEndpoints, nodeStatus.VMComputerName)
	}

	startupScript := getMasterStartupScript(
		instance.Spec.KubernetesVersion,
		cluster.Spec.PublicIPAdress,
		cluster.Spec.InternalDNSName,
		bootstrapToken,
		cluster.Spec.DiscoveryHashes[0],
		etcdEndpoints,
	)

	customRunData := map[string]string{
		"/etc/kubernetes/init-azure-bootstrap.sh": startupScript,
	}

	prefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers",
		cluster.Spec.SubscriptionID,
		cluster.Spec.GroupName)

	subnetID := prefix + "/Microsoft.Network/virtualNetworks/azk-vnet/subnets/master-subnet"

	loadbalancerIDs := []string{
		prefix + "/Microsoft.Network/loadBalancers/azk-lb/backendAddressPools/master-backEndPool",
		prefix + "/Microsoft.Network/loadBalancers/azk-internal-lb/backendAddressPools/master-internal-backEndPool",
	}

	natPoolIDs := []string{
		prefix + "/Microsoft.Network/loadBalancers/azk-lb/inboundNatPools/natSSHPool",
	}

	log.Info("Creating or Updating", "VMSS", masterVmssName)
	if err := cluster.Spec.CreateVMSS(
		ctx,
		masterVmssName,
		subnetID,
		loadbalancerIDs,
		natPoolIDs,
		base64.StdEncoding.EncodeToString([]byte(azhelpers.GetCustomData(customData, customRunData))),
		vmSKUType,
		3); err != nil {
		return ctrl.Result{}, err
	}
	log.Info("Successfully Created or Updated", "VMSS", masterVmssName)
	if instance.Status.KubernetesVersion != "" &&
		instance.Status.KubernetesVersion != instance.Spec.KubernetesVersion {
		if err := r.upgradeVMSS(instance, cluster); err != nil {
			return ctrl.Result{}, err
		}
	}
	if err := helpers.WaitForNodesReady(r.Client, masterVmssName, 3); err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
	}

	instance.Status.KubernetesVersion = instance.Spec.KubernetesVersion
	instance.Status.ProvisioningState = "Succeeded"
	if err := r.Status().Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(instance, "Normal", "Created", fmt.Sprintf("Control Plane"))

	return ctrl.Result{}, nil
}

func (r *ControlPlaneReconciler) getCluster(ctx context.Context, namespace string) (*enginev1alpha1.Cluster, error) {
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

func (r *ControlPlaneReconciler) updateVMSSStatus(instance *enginev1alpha1.ControlPlane, cluster *enginev1alpha1.Cluster) error {
	ctx := context.Background()
	log := r.Log.WithValues("controlplane", instance.Name)

	vmssVMClient, err := cluster.Spec.GetVMSSVMsClient()
	if err != nil {
		log.Error(err, "Error GetVMSSVMsClient", "VMSS", masterVmssName)
		return err
	}

	result, err := vmssVMClient.List(ctx, cluster.Spec.GroupName, masterVmssName, "", "", string(compute.InstanceView))
	if err != nil {
		log.Error(err, "Error vmssVMClient List", "VMSS", masterVmssName)
		return err
	}

	var vmStatus []enginev1alpha1.VMStatus
	for _, vmID := range result.Values() {
		log.Info("Appending to VMSS ControlPlane list", "VM", *vmID.OsProfile.ComputerName)
		var status enginev1alpha1.VMStatus
		status.VMComputerName = *vmID.OsProfile.ComputerName
		status.VMInstanceID = *vmID.InstanceID
		vmStatus = append(vmStatus, status)
	}
	instance.Status.NodeStatus = vmStatus
	return nil
}

func (r *ControlPlaneReconciler) upgradeVMSS(instance *enginev1alpha1.ControlPlane, cluster *enginev1alpha1.Cluster) error {
	ctx := context.Background()
	log := r.Log.WithValues("controlplane", instance.Name)

	vmssVMClient, err := cluster.Spec.GetVMSSVMsClient()
	if err != nil {
		log.Error(err, "Error GetVMSSVMsClient", "VMSS", masterVmssName)
		return err
	}

	upgradeCommand := compute.RunCommandInput{
		CommandID: to.StringPtr("RunShellScript"),
		Script: &[]string{
			getUpgradeScript(instance),
		},
	}

	for _, nodeStatus := range instance.Status.NodeStatus {
		if isUpdated, err := helpers.IsNodeUpdated(r.Client, nodeStatus.VMComputerName, instance.Spec.KubernetesVersion); err != nil {
			log.Error(err, "Error checking upgrade version", "VM", nodeStatus.VMComputerName)
			return err
		} else if isUpdated {
			log.Info("Node Already at Expected Kubernetes Version", "VM", nodeStatus.VMComputerName)
			continue
		}

		log.Info("Running Custom Script Extension, Upgrading", "VM", nodeStatus.VMComputerName, "KubernetesVersion", instance.Spec.KubernetesVersion)

		future, err := vmssVMClient.RunCommand(ctx, cluster.Spec.GroupName, masterVmssName, nodeStatus.VMInstanceID, upgradeCommand)
		if err != nil {
			log.Error(err, "Error vmssVMClient Upgrade", "VMSS", masterVmssName)
			return err
		}
		err = future.WaitForCompletionRef(ctx, vmssVMClient.Client)
		if err != nil {
			return fmt.Errorf("cannot get the vmss update future response: %v", err)
		}

		_, err = future.Result(vmssVMClient)
		if err != nil {
			log.Error(err, "Error Upgrading", "VMSS", masterVmssName, "VM", nodeStatus.VMComputerName)
			return err
		}

		if err := helpers.WaitForNodeVersionReady(r.Client, nodeStatus.VMComputerName, instance.Spec.KubernetesVersion); err != nil {
			log.Error(err, "Error waiting for upgrade", "VMSS", masterVmssName, "VM", nodeStatus.VMComputerName)
			return err
		}

		log.Info("Successfully Executed Custom Script Extension", "VM", nodeStatus.VMComputerName, "KubernetesVersion", instance.Spec.KubernetesVersion)
	}

	return nil
}

func (r *ControlPlaneReconciler) upgradeVMSSWithReimage(instance *enginev1alpha1.ControlPlane, cluster *enginev1alpha1.Cluster) error {
	ctx := context.Background()
	log := r.Log.WithValues("controlplane", instance.Name)

	vmssVMClient, err := cluster.Spec.GetVMSSVMsClient()
	if err != nil {
		log.Error(err, "Error GetVMSSVMsClient", "VMSS", masterVmssName)
		return err
	}

	for _, nodeStatus := range instance.Status.NodeStatus {
		if isUpdated, err := helpers.IsNodeUpdated(r.Client, nodeStatus.VMComputerName, instance.Spec.KubernetesVersion); err != nil {
			log.Error(err, "Error checking upgrade version", "VM", nodeStatus.VMComputerName)
			return err
		} else if isUpdated {
			log.Info("Node Already at Expected Kubernetes Version", "VM", nodeStatus.VMComputerName)
			continue
		}

		log.Info("Cordon, Drain and Delete Node", "VM", nodeStatus.VMComputerName, "KubernetesVersion", instance.Spec.KubernetesVersion)
		if err := helpers.CordonDrainAndDeleteNode(cluster.Spec.CustomerKubeConfig, nodeStatus.VMComputerName); err != nil {
			log.Info("Error in Cordon and Drain", "Error", err, "VM", nodeStatus.VMComputerName)
		}

		log.Info("Upgrading using Reimage", "VM", nodeStatus.VMComputerName, "KubernetesVersion", instance.Spec.KubernetesVersion)

		future, err := vmssVMClient.Reimage(ctx, cluster.Spec.GroupName, masterVmssName, nodeStatus.VMInstanceID, nil)
		if err != nil {
			log.Error(err, "Error vmssVMClient Upgrade", "VMSS", masterVmssName)
			return err
		}
		err = future.WaitForCompletionRef(ctx, vmssVMClient.Client)
		if err != nil {
			return fmt.Errorf("cannot get the vmss update future response: %v", err)
		}

		_, err = future.Result(vmssVMClient)
		if err != nil {
			log.Error(err, "Error Upgrading", "VMSS", masterVmssName, "VM", nodeStatus.VMComputerName)
			return err
		}

		if err := helpers.WaitForNodeVersionReady(r.Client, nodeStatus.VMComputerName, instance.Spec.KubernetesVersion); err != nil {
			log.Error(err, "Error waiting for upgrade", "VMSS", masterVmssName, "VM", nodeStatus.VMComputerName)
			return err
		}

		log.Info("Successfully Upgraded using Reimage", "VM", nodeStatus.VMComputerName, "KubernetesVersion", instance.Spec.KubernetesVersion)
	}

	return nil
}

func (r *ControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&enginev1alpha1.ControlPlane{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 30}).
		Complete(r)
}
