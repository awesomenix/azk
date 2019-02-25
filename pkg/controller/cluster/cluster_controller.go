package cluster

import (
	"context"
	"io/ioutil"
	"time"

	enginev1alpha1 "github.com/awesomenix/azkube/pkg/apis/engine/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	kubeadmscheme "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/scheme"
	kubeadmv1beta1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta1"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	certsphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/certs"
	kubeconfigphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	tmpDir = "/tmp/kubeadm/"
)

var log = logf.Log.WithName("controller")

// Add creates a new Cluster Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCluster{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cluster-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Cluster
	err = c.Watch(&source.Kind{Type: &enginev1alpha1.Cluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileCluster{}

// ReconcileCluster reconciles a Cluster object
type ReconcileCluster struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Cluster object and makes changes based on the state read
// and what is in the Cluster.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=engine.azkube.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=engine.azkube.io,resources=clusters/status,verbs=get;update;patch
func (r *ReconcileCluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Cluster instance
	instance := &enginev1alpha1.Cluster{}
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

	instance.Status.ProvisioningState = "Updating"
	if err := r.Status().Update(context.TODO(), instance); err != nil {
		return reconcile.Result{}, err
	}

	v1beta1cfg := &kubeadmv1beta1.InitConfiguration{}
	kubeadmscheme.Scheme.Default(v1beta1cfg)
	v1beta1cfg.CertificatesDir = tmpDir + request.Name + "/certs"
	v1beta1cfg.Etcd.Local = &kubeadmv1beta1.LocalEtcd{}
	v1beta1cfg.LocalAPIEndpoint = kubeadmv1beta1.APIEndpoint{AdvertiseAddress: "10.0.0.1", BindPort: 6443}
	v1beta1cfg.NodeRegistration.Name = "fakenode" + request.Name
	cfg := &kubeadmapi.InitConfiguration{}
	kubeadmscheme.Scheme.Default(cfg)
	kubeadmscheme.Scheme.Convert(v1beta1cfg, cfg, nil)

	if err := createCertificates(cfg); err != nil {
		log.Error(err, "Error Generating Certificates")
		return reconcile.Result{RequeueAfter: 3 * time.Second}, err
	}

	kubeConfigDir := tmpDir + request.Name + "/kubeconfigs"
	if err := createKubeconfigs(cfg, kubeConfigDir); err != nil {
		log.Error(err, "Error Generating Kubeconfigs")
		return reconcile.Result{RequeueAfter: 3 * time.Second}, err
	}

	if err := updateStatus(instance); err != nil {
		log.Error(err, "Error Updating Status")
		return reconcile.Result{RequeueAfter: 3 * time.Second}, err
	}
	instance.Status.ProvisioningState = "Succeeded"
	if err := r.Status().Update(context.TODO(), instance); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func createCertificates(cfg *kubeadmapi.InitConfiguration) error {
	if err := certsphase.CreatePKIAssets(cfg); err != nil {
		return err
	}

	if err := certsphase.CreateServiceAccountKeyAndPublicKeyFiles(cfg); err != nil {
		return err
	}

	return nil
}

func createKubeconfigs(cfg *kubeadmapi.InitConfiguration, kubeConfigDir string) error {
	if err := kubeconfigphase.CreateKubeConfigFile(kubeadmconstants.AdminKubeConfigFileName, kubeConfigDir, cfg); err != nil {
		return err
	}
	if err := kubeconfigphase.CreateKubeConfigFile(kubeadmconstants.KubeletKubeConfigFileName, kubeConfigDir, cfg); err != nil {
		return err
	}
	if err := kubeconfigphase.CreateKubeConfigFile(kubeadmconstants.ControllerManagerKubeConfigFileName, kubeConfigDir, cfg); err != nil {
		return err
	}
	if err := kubeconfigphase.CreateKubeConfigFile(kubeadmconstants.SchedulerKubeConfigFileName, kubeConfigDir, cfg); err != nil {
		return err
	}
	return nil
}

func updateStatus(instance *enginev1alpha1.Cluster) error {
	buf, err := ioutil.ReadFile(tmpDir + instance.Name + "/certs/ca.crt")
	if err != nil {
		return err
	}
	instance.Status.CACertificate = string(buf)
	buf, err = ioutil.ReadFile(tmpDir + instance.Name + "/certs/ca.key")
	if err != nil {
		return err
	}
	instance.Status.CACertificateKey = string(buf)
	buf, err = ioutil.ReadFile(tmpDir + instance.Name + "/certs/sa.key")
	if err != nil {
		return err
	}
	instance.Status.ServiceAccountKey = string(buf)
	buf, err = ioutil.ReadFile(tmpDir + instance.Name + "/certs/sa.pub")
	if err != nil {
		return err
	}
	instance.Status.ServiceAccountPub = string(buf)
	buf, err = ioutil.ReadFile(tmpDir + instance.Name + "/certs/front-proxy-ca.crt")
	if err != nil {
		return err
	}
	instance.Status.FrontProxyCACertificate = string(buf)
	buf, err = ioutil.ReadFile(tmpDir + instance.Name + "/certs/front-proxy-ca.key")
	if err != nil {
		return err
	}
	instance.Status.FrontProxyCACertificateKey = string(buf)
	buf, err = ioutil.ReadFile(tmpDir + instance.Name + "/certs/etcd/ca.crt")
	if err != nil {
		return err
	}
	instance.Status.EtcdCACertificate = string(buf)
	buf, err = ioutil.ReadFile(tmpDir + instance.Name + "/certs/etcd/ca.key")
	if err != nil {
		return err
	}
	instance.Status.EtcdCACertificateKey = string(buf)
	buf, err = ioutil.ReadFile(tmpDir + instance.Name + "/kubeconfigs/admin.conf")
	if err != nil {
		return err
	}
	instance.Status.AdminKubeConfig = string(buf)
	return nil
}
