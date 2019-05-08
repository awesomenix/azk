package controlplane

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/Azure/go-autorest/autorest/to"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-03-01/compute"
	enginev1alpha1 "github.com/awesomenix/azk/pkg/apis/engine/v1alpha1"
	azhelpers "github.com/awesomenix/azk/pkg/azure"
	"github.com/awesomenix/azk/pkg/bootstrap"
	"github.com/awesomenix/azk/pkg/helpers"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller")

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

func kubeadmJoinConfig(bootstrapToken, internalDNSName, discoveryHash string) string {
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

func getMasterStartupScript(kubernetesVersion, apiServerIP, internalDNSName, bootstrapToken, discoveryHash string) string {
	return fmt.Sprintf(`
set -eux
%[1]s
%[2]s
#Setup using kubeadm
sudo kubeadm join --config /tmp/kubeadm-config.yaml
sudo cp -f /etc/hosts.bak /tmp/hostsupdate
sudo chown $(id -u):$(id -g) /tmp/hostsupdate
echo '127.0.0.1 %[3]s' >> /tmp/hostsupdate
sudo mv /tmp/hostsupdate /etc/hosts
`, preRequisites(kubernetesVersion, apiServerIP, internalDNSName),
		kubeadmJoinConfig(bootstrapToken, internalDNSName, discoveryHash),
		internalDNSName,
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

// Add creates a new ControlPlane Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	r := &ReconcileControlPlane{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
	recorder := mgr.GetRecorder("contolplane-controller")
	r.recorder = recorder
	return r
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("controlplane-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: 30})
	if err != nil {
		return err
	}

	// Watch for changes to ControlPlane
	err = c.Watch(&source.Kind{Type: &enginev1alpha1.ControlPlane{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileControlPlane{}

// ReconcileControlPlane reconciles a ControlPlane object
type ReconcileControlPlane struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile reads that state of the cluster for a ControlPlane object and makes changes based on the state read
// and what is in the ControlPlane.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=engine.azk.io,resources=controlplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=engine.azk.io,resources=controlplanes/status,verbs=get;update;patch
func (r *ReconcileControlPlane) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer helpers.Recover()
	// Fetch the ControlPlane instance
	instance := &enginev1alpha1.ControlPlane{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if instance.Spec.KubernetesVersion == instance.Status.KubernetesVersion {
		return reconcile.Result{}, nil
	}

	cluster, err := r.getCluster(context.TODO(), instance.Namespace)
	if err != nil {
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
	}

	if cluster.Status.ProvisioningState != "Succeeded" {
		// Wait for cluster to initialize
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
	}

	if err := updateVMSSStatus(instance, cluster); err != nil {
		return reconcile.Result{}, err
	}

	if instance.Status.ProvisioningState == "Succeeded" &&
		instance.Spec.KubernetesVersion == instance.Status.KubernetesVersion {
		return reconcile.Result{}, nil
	}

	log.Info("Updating Control Plane",
		"CurrentKubernetesVersion", instance.Status.KubernetesVersion,
		"ExpectedKubernetesVersion", instance.Spec.KubernetesVersion)

	instance.Status.ProvisioningState = "Updating"
	if err := r.Status().Update(context.TODO(), instance); err != nil {
		return reconcile.Result{}, err
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
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
	}

	startupScript := getMasterStartupScript(
		instance.Spec.KubernetesVersion,
		"10.0.0.4",
		cluster.Spec.InternalDNSName,
		bootstrapToken,
		cluster.Spec.DiscoveryHashes[0])

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
		context.TODO(),
		masterVmssName,
		subnetID,
		loadbalancerIDs,
		natPoolIDs,
		base64.StdEncoding.EncodeToString([]byte(azhelpers.GetCustomData(customData, customRunData))),
		vmSKUType,
		3); err != nil {
		return reconcile.Result{}, err
	}
	log.Info("Successfully Created or Updated", "VMSS", masterVmssName)
	if instance.Status.KubernetesVersion != "" &&
		instance.Status.KubernetesVersion != instance.Spec.KubernetesVersion {
		if err := r.upgradeVMSS(instance, cluster); err != nil {
			return reconcile.Result{}, err
		}
	}
	if err := helpers.WaitForNodesReady(r.Client, masterVmssName, 3); err != nil {
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
	}

	instance.Status.KubernetesVersion = instance.Spec.KubernetesVersion
	instance.Status.ProvisioningState = "Succeeded"
	if err := r.Status().Update(context.TODO(), instance); err != nil {
		return reconcile.Result{}, err
	}

	r.recorder.Event(instance, "Normal", "Created", fmt.Sprintf("Control Plane"))

	return reconcile.Result{}, nil
}

func (r *ReconcileControlPlane) getCluster(ctx context.Context, namespace string) (*enginev1alpha1.Cluster, error) {
	clusterList := enginev1alpha1.ClusterList{}
	listOptions := &client.ListOptions{
		Namespace: namespace,
		Raw: &metav1.ListOptions{
			TypeMeta: metav1.TypeMeta{
				APIVersion: enginev1alpha1.SchemeGroupVersion.String(),
				Kind:       "Cluster",
			},
		},
	}

	if err := r.Client.List(ctx, listOptions, &clusterList); err != nil {
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

func updateVMSSStatus(instance *enginev1alpha1.ControlPlane, cluster *enginev1alpha1.Cluster) error {
	vmssVMClient, err := cluster.Spec.GetVMSSVMsClient()
	if err != nil {
		log.Error(err, "Error GetVMSSVMsClient", "VMSS", masterVmssName)
		return err
	}

	result, err := vmssVMClient.List(context.TODO(), cluster.Spec.GroupName, masterVmssName, "", "", string(compute.InstanceView))
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

func (r *ReconcileControlPlane) upgradeVMSS(instance *enginev1alpha1.ControlPlane, cluster *enginev1alpha1.Cluster) error {
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
		log.Info("Running Custom Script Extension, Upgrading", "VM", nodeStatus.VMComputerName, "KubernetesVersion", instance.Spec.KubernetesVersion)

		future, err := vmssVMClient.RunCommand(context.TODO(), cluster.Spec.GroupName, masterVmssName, nodeStatus.VMInstanceID, upgradeCommand)
		if err != nil {
			log.Error(err, "Error vmssVMClient Upgrade", "VMSS", masterVmssName)
			return err
		}
		err = future.WaitForCompletionRef(context.TODO(), vmssVMClient.Client)
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
