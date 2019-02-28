package controlplane

import (
	"context"
	"encoding/base64"
	"fmt"
	debugruntime "runtime"
	"runtime/debug"
	"sync"
	"time"

	enginev1alpha1 "github.com/awesomenix/azkube/pkg/apis/engine/v1alpha1"
	azhelpers "github.com/awesomenix/azkube/pkg/azure"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	masterAvailabilitySetName = "azkube-masters-availabilityset"
)

func getEncodedPrimaryMasterStartupScript(bootstrapToken, publicIPAddress string) string {
	startupScript := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(`
sudo apt-get update && sudo apt-get install -y apt-transport-https ca-certificates curl gnupg-agent software-properties-common
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
sudo add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"
sudo apt-get install -y docker-ce=18.06.0~ce~3-0~ubuntu containerd.io
curl -fsSL https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -
cat <<EOF >/tmp/kubernetes.list
deb https://apt.kubernetes.io/ kubernetes-xenial main
EOF
sudo mv /tmp/kubernetes.list /etc/apt/sources.list.d/kubernetes.list
sudo apt-get update && sudo apt-get install -y kubelet kubeadm kubectl
sudo apt-mark hold kubelet kubeadm kubectl
cat <<EOF >/tmp/kubeadm-config.yaml
apiVersion: kubeadm.k8s.io/v1beta1
bootstrapTokens:
- groups:
  - system:bootstrappers:kubeadm:default-node-token
  token: %[1]s
  ttl: 48h0m0s
  usages:
  - signing
  - authentication
kind: InitConfiguration
---
apiVersion: kubeadm.k8s.io/v1beta1
kind: ClusterConfiguration
apiServer:
  certSANs:
  - "%[2]s"
kubernetesVersion: stable
controlPlaneEndpoint: "192.0.0.4:6443"
networking:
  podSubnet: "192.168.0.0/16"
EOF
#Setup using kubeadm
sudo kubeadm config images pull
#flannel
#sudo sysctl net.bridge.bridge-nf-call-iptables=1
#use 10.244.0.0/16 as podsubnet above
#sudo kubeadm init --config /tmp/kubeadm-config.yaml
#calico
sudo kubeadm init --config /tmp/kubeadm-config.yaml
mkdir -p $HOME/.kube
sudo cp -f /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
#calico
sudo kubectl --kubeconfig /etc/kubernetes/admin.conf apply -f https://docs.projectcalico.org/v3.5/getting-started/kubernetes/installation/hosted/etcd.yaml
sudo kubectl --kubeconfig /etc/kubernetes/admin.conf apply -f https://docs.projectcalico.org/v3.5/getting-started/kubernetes/installation/hosted/calico.yaml
#flannel
#sudo kubectl --kubeconfig /etc/kubernetes/admin.conf apply -f https://raw.githubusercontent.com/coreos/flannel/master/Documentation/kube-flannel.yml
`, bootstrapToken, publicIPAddress)))
	return startupScript
}

func getEncodedSecondaryMasterStartupScript(bootstrapToken, discoveryHash string) string {
	startupScript := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(`
sudo apt-get update && sudo apt-get install -y apt-transport-https ca-certificates curl gnupg-agent software-properties-common
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
sudo add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"
sudo apt-get install -y docker-ce=18.06.0~ce~3-0~ubuntu containerd.io
curl -fsSL https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -
cat <<EOF >/tmp/kubernetes.list
deb https://apt.kubernetes.io/ kubernetes-xenial main
EOF
sudo mv /tmp/kubernetes.list /etc/apt/sources.list.d/kubernetes.list
sudo apt-get update && sudo apt-get install -y kubelet kubeadm kubectl
sudo apt-mark hold kubelet kubeadm kubectl
#Setup using kubeadm
sudo kubeadm config images pull
sudo kubeadm join 192.0.0.4:6443 --token %[1]s --discovery-token-ca-cert-hash %[2]s --experimental-control-plane
`, bootstrapToken, discoveryHash)))
	return startupScript
}

// Add creates a new ControlPlane Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileControlPlane{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
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
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ControlPlane object and makes changes based on the state read
// and what is in the ControlPlane.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=engine.azkube.io,resources=controlplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=engine.azkube.io,resources=controlplanes/status,verbs=get;update;patch
func (r *ReconcileControlPlane) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer func() {
		// recover from panic if one occured. Set err to nil otherwise.
		if r := recover(); r != nil {
			_, file, line, _ := debugruntime.Caller(3)
			stack := string(debug.Stack())
			log.Error(fmt.Errorf("Panic: %+v, file: %s, line: %d, stacktrace: '%s'", r, file, line, stack), "Panic Observed")
		}
	}()
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
		return reconcile.Result{}, err
	}

	if cluster.Status.ProvisioningState != "Succeeded" {
		// Wait for cluster to initialize
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
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

	cloudConfig := azhelpers.CloudConfiguration{
		CloudName:      azhelpers.AzurePublicCloudName,
		SubscriptionID: cluster.Spec.SubscriptionID,
		ClientID:       cluster.Spec.ClientID,
		ClientSecret:   cluster.Spec.ClientSecret,
		TenantID:       cluster.Spec.TenantID,
		GroupName:      cluster.Spec.ResourceGroupName,
		GroupLocation:  cluster.Spec.Location,
		UserAgent:      "azkube",
	}

	customData := map[string]string{
		"/etc/kubernetes/pki/ca.crt":             cluster.Status.CACertificate,
		"/etc/kubernetes/pki/ca.key":             cluster.Status.CACertificateKey,
		"/etc/kubernetes/pki/sa.key":             cluster.Status.ServiceAccountKey,
		"/etc/kubernetes/pki/sa.pub":             cluster.Status.ServiceAccountPub,
		"/etc/kubernetes/pki/front-proxy-ca.crt": cluster.Status.FrontProxyCACertificate,
		"/etc/kubernetes/pki/front-proxy-ca.key": cluster.Status.FrontProxyCACertificateKey,
		"/etc/kubernetes/pki/etcd/ca.crt":        cluster.Status.EtcdCACertificate,
		"/etc/kubernetes/pki/etcd/ca.key":        cluster.Status.EtcdCACertificateKey,
		"/etc/kubernetes/admin.conf":             cluster.Status.AdminKubeConfig,
	}

	log.Info("Creating", "AvailabilitySet", masterAvailabilitySetName)
	if _, err := cloudConfig.CreateAvailabilitySet(
		context.TODO(),
		masterAvailabilitySetName); err != nil {
		return reconcile.Result{}, err
	}
	log.Info("Successfully Created", "AvailabilitySet", masterAvailabilitySetName)

	var globalErr error
	{
		var wg sync.WaitGroup
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(vmIndex int) {
				defer wg.Done()
				vmName := fmt.Sprintf("%s-mastervm-%d", instance.Name, vmIndex)
				log.Info("Creating", "VM", vmName)
				if err := cloudConfig.CreateVMWithLoadBalancer(
					context.TODO(),
					vmName,
					"azkube-lb",
					"azkube-vnet",
					"master-subnet",
					fmt.Sprintf("192.0.0.%d", vmIndex+4),
					azhelpers.GetCustomData(customData),
					masterAvailabilitySetName,
					vmIndex); err != nil {
					log.Error(err, "Creation Failed", "VM", vmName)
					globalErr = err
					return
				}
				log.Info("Successfully Created", "VM", vmName)
			}(i)
		}
		wg.Wait()
	}

	if globalErr != nil {
		return reconcile.Result{}, globalErr
	}

	vmName := fmt.Sprintf("%s-mastervm-%d", instance.Name, 0)
	log.Info("Running Custom Script Extension", "VM", vmName)
	if err := cloudConfig.AddCustomScriptsExtension(
		context.TODO(),
		vmName,
		getEncodedPrimaryMasterStartupScript(cluster.Status.BootstrapToken, cluster.Status.PublicIPAddress)); err != nil {
		log.Error(err, "Error Executing Custom Script Extension", "VM", vmName)
		return reconcile.Result{}, err
	}
	log.Info("Successfully Executed Custom Script Extension", "VM", vmName)

	{
		var wg sync.WaitGroup
		for i := 1; i < 3; i++ {
			wg.Add(1)
			go func(vmIndex int) {
				vmName = fmt.Sprintf("%s-mastervm-%d", instance.Name, vmIndex)
				log.Info("Running Custom Script Extension", "VM", vmName)
				if err := cloudConfig.AddCustomScriptsExtension(
					context.TODO(),
					vmName,
					getEncodedSecondaryMasterStartupScript(cluster.Status.BootstrapToken, cluster.Status.DiscoveryHashes[0])); err != nil {
					log.Error(err, "Error Executing Custom Script Extension", "VM", vmName)
					globalErr = err
					return
				}
				log.Info("Successfully Executed Custom Script Extension", "VM", vmName)
			}(i)
		}
		wg.Wait()
	}
	if globalErr != nil {
		return reconcile.Result{}, globalErr
	}

	instance.Status.KubernetesVersion = instance.Spec.KubernetesVersion
	instance.Status.ProvisioningState = "Succeeded"
	if err := r.Status().Update(context.TODO(), instance); err != nil {
		return reconcile.Result{}, err
	}

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
