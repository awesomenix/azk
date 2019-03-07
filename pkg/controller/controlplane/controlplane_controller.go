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
	"github.com/awesomenix/azkube/pkg/helpers"
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
	controlPlaneFinalizerName = "controlplane.finalizers.engine.azkube.io"
)

func preRequisites(cluster *enginev1alpha1.Cluster, instance *enginev1alpha1.ControlPlane) string {
	return fmt.Sprintf(`
%[1]s
sudo cp -f /etc/hosts /tmp/hostsupdate
sudo chown $(id -u):$(id -g) /tmp/hostsupdate
echo '192.0.0.4 %[2]s' >> /tmp/hostsupdate
sudo mv /etc/hosts /etc/hosts.bak
sudo mv /tmp/hostsupdate /etc/hosts
`, helpers.PreRequisitesInstallScript(instance.Spec.KubernetesVersion), cluster.Spec.InternalDNSName)
}

func kubeadmJoinConfig(cluster *enginev1alpha1.Cluster) string {
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
`, cluster.Spec.BootstrapToken,
		cluster.Spec.InternalDNSName,
		cluster.Spec.DiscoveryHashes[0],
	)
}

func getSecondaryMasterStartupScript(cluster *enginev1alpha1.Cluster, instance *enginev1alpha1.ControlPlane) string {
	return fmt.Sprintf(`
%[1]s
%[2]s
#Setup using kubeadm
sudo kubeadm join --config /tmp/kubeadm-config.yaml
sudo cp -f /etc/hosts.bak /tmp/hostsupdate
sudo chown $(id -u):$(id -g) /tmp/hostsupdate
echo '127.0.0.1 %[3]s' >> /tmp/hostsupdate
sudo mv /tmp/hostsupdate /etc/hosts
`, preRequisites(cluster, instance),
		kubeadmJoinConfig(cluster),
		cluster.Spec.InternalDNSName,
	)
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

	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object.
		update := false
		if !helpers.ContainsFinalizer(instance.ObjectMeta.Finalizers, controlPlaneFinalizerName) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, controlPlaneFinalizerName)
			update = true
		}

		if update {
			if err := r.Update(context.TODO(), instance); err != nil {
				return reconcile.Result{Requeue: true}, err
			}
			// Once updates object changes we need to requeue
			return reconcile.Result{Requeue: true}, nil
		}

	} else {
		if helpers.ContainsFinalizer(instance.ObjectMeta.Finalizers, controlPlaneFinalizerName) {
			cluster, err := r.getCluster(context.TODO(), instance.Namespace)

			if err == nil && cluster.Spec.IsValid() {
				// resourceName := fmt.Sprintf("%s-mastervm", instance.Name)
				// log.Info("Deleting Resources", "Name", resourceName)
				// our finalizer is present, so lets handle our external dependency
				// if err := cluster.Spec.DeleteResources(context.TODO(), resourceName); err != nil {
				// 	// if fail to delete the external dependency here, return with error
				// 	// so that it can be retried
				// 	// meh! its fine if it fails, we definitely need to wait here for it to be deleted
				// 	log.Error(err, "Error Deleting Resources", "Name", resourceName)
				// } else {
				// 	log.Info("Successfully Deleted Resources", "Name", resourceName)
				// }
			}

			// remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = helpers.RemoveFinalizer(instance.ObjectMeta.Finalizers, controlPlaneFinalizerName)
			if err := r.Update(context.TODO(), instance); err != nil {
				return reconcile.Result{Requeue: true}, nil
			}
		}

		return reconcile.Result{}, nil
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

	log.Info("Creating", "AvailabilitySet", masterAvailabilitySetName)
	if _, err := cluster.Spec.CreateAvailabilitySet(
		context.TODO(),
		masterAvailabilitySetName); err != nil {
		return reconcile.Result{}, err
	}
	log.Info("Successfully Created", "AvailabilitySet", masterAvailabilitySetName)

	vmSKUType := instance.Spec.VMSKUType
	if vmSKUType == "" {
		vmSKUType = "Standard_DS2_v2"
	}

	var globalErr error
	{
		var wg sync.WaitGroup
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(vmIndex int) {
				defer wg.Done()
				customRunData := map[string]string{
					"/etc/kubernetes/init-azure-bootstrap.sh": cluster.Spec.GetPrimaryMasterStartupScript(instance.Spec.KubernetesVersion),
				}
				if vmIndex > 0 {
					customRunData = map[string]string{
						"/etc/kubernetes/init-azure-bootstrap.sh": getSecondaryMasterStartupScript(cluster, instance),
					}
				}
				vmName := fmt.Sprintf("%s-mastervm-%d", instance.Name, vmIndex)
				log.Info("Creating", "VM", vmName)
				if err := cluster.Spec.CreateVMWithLoadBalancer(
					context.TODO(),
					vmName,
					"azkube-lb",
					"azkube-internal-lb",
					"azkube-vnet",
					"master-subnet",
					fmt.Sprintf("192.0.0.%d", vmIndex+4),
					base64.StdEncoding.EncodeToString([]byte(azhelpers.GetCustomData(customData, customRunData))),
					masterAvailabilitySetName,
					vmSKUType,
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
