package cluster

import (
	"context"
	"crypto/x509"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"os"
	debugruntime "runtime"
	"runtime/debug"
	"strings"
	"time"

	enginev1alpha1 "github.com/awesomenix/azkube/pkg/apis/engine/v1alpha1"
	azhelpers "github.com/awesomenix/azkube/pkg/azure"
	"github.com/awesomenix/azkube/pkg/helpers"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientcertutil "k8s.io/client-go/util/cert"
	bootstraputil "k8s.io/cluster-bootstrap/token/util"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	kubeadmscheme "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/scheme"
	kubeadmv1beta1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta1"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	certsphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/certs"
	kubeconfigphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/kubeconfig"
	kubeconfigutil "k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/pubkeypin"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	tmpDir                         = "/tmp/kubeadm/"
	groupsFinalizerName            = "groups.finalizers.engine.azkube.io"
	azkubeLoadBalancerName         = "azkube-lb"
	azkubeInternalLoadBalancerName = "azkube-internal-lb"
	azkubePublicIPName             = "azkube-publicip"
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
	c, err := controller.New("cluster-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: 30})
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
	defer func() {
		// recover from panic if one occured. Set err to nil otherwise.
		if r := recover(); r != nil {
			_, file, line, _ := debugruntime.Caller(3)
			stack := string(debug.Stack())
			log.Error(fmt.Errorf("Panic: %+v, file: %s, line: %d, stacktrace: '%s'", r, file, line, stack), "Panic Observed")
		}
	}()
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

	cloudConfig := azhelpers.CloudConfiguration{
		CloudName:      azhelpers.AzurePublicCloudName,
		SubscriptionID: instance.Spec.SubscriptionID,
		ClientID:       instance.Spec.ClientID,
		ClientSecret:   instance.Spec.ClientSecret,
		TenantID:       instance.Spec.TenantID,
		GroupName:      instance.Spec.ResourceGroupName,
		GroupLocation:  instance.Spec.Location,
		UserAgent:      "azkube",
	}

	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object.
		update := false

		if !helpers.ContainsFinalizer(instance.ObjectMeta.Finalizers, groupsFinalizerName) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, groupsFinalizerName)
			if cloudConfig.IsValid() {
				log.Info("Creating", "ResourceGroup", instance.Spec.ResourceGroupName, "Location", instance.Spec.Location)
				err := cloudConfig.CreateOrUpdateResourceGroup(context.TODO())
				if err != nil {
					return reconcile.Result{Requeue: true}, err
				}
				log.Info("Successfully Created", "ResourceGroup", instance.Spec.ResourceGroupName, "Location", instance.Spec.Location)
				log.Info("Creating", "VNET", "azkube-vnet", "Location")
				err = cloudConfig.CreateVirtualNetworkAndSubnets(context.TODO(), "azkube-vnet")
				if err != nil {
					return reconcile.Result{Requeue: true}, err
				}
				log.Info("Successfully Created", "VNET", "azkube-vnet", "Location")
			}
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
		if helpers.ContainsFinalizer(instance.ObjectMeta.Finalizers, groupsFinalizerName) {
			if cloudConfig.IsValid() {
				log.Info("Deleting Resource Group", "Name", instance.Spec.ResourceGroupName)
				// our finalizer is present, so lets handle our external dependency
				if err := cloudConfig.DeleteResourceGroup(context.TODO()); err != nil {
					// if fail to delete the external dependency here, return with error
					// so that it can be retried
					// meh! its fine if it fails, we definitely need to wait here for it to be deleted
					log.Error(err, "Error Deleting Resource Group", "Name", instance.Spec.ResourceGroupName)
				} else {
					log.Info("Successfully Deleted Resource Group", "Name", instance.Spec.ResourceGroupName)
				}
			}

			// remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = helpers.RemoveFinalizer(instance.ObjectMeta.Finalizers, groupsFinalizerName)
			if err := r.Update(context.TODO(), instance); err != nil {
				return reconcile.Result{Requeue: true}, nil
			}
		}

		return reconcile.Result{}, nil
	}

	if instance.Status.ProvisioningState == "Succeeded" {
		// Ideally we need to check that everything is good
		return reconcile.Result{}, nil
	}

	instance.Status.ProvisioningState = "Updating"
	if err := r.Status().Update(context.TODO(), instance); err != nil {
		return reconcile.Result{}, err
	}

	h := fnv.New32a()
	h.Write([]byte(fmt.Sprintf("%s-%s", azkubePublicIPName, instance.Name)))
	publicIPName := fmt.Sprintf("%x", h.Sum32())
	publicIPName = instance.Spec.DNSPrefix + publicIPName

	publicAddress := ""
	publicDNSName := fmt.Sprintf("%s.%s.cloudapp.azure.com", strings.ToLower(publicIPName), strings.ToLower(instance.Spec.Location))
	internalDNSName := fmt.Sprintf("%s.internal", strings.ToLower(publicIPName))
	if cloudConfig.IsValid() {
		log.Info("Creating Internal Load Balancer", "Name", azkubeInternalLoadBalancerName)
		if err := cloudConfig.CreateInternalLoadBalancer(
			context.TODO(),
			"azkube-vnet",
			"master-subnet",
			azkubeInternalLoadBalancerName); err != nil {
			return reconcile.Result{}, err
		}
		log.Info("Successfully Created Internal Load Balancer", "Name", azkubeInternalLoadBalancerName)

		log.Info("Creating Public Load Balancer", "Name", azkubeLoadBalancerName, "PublicIPName", publicIPName)
		if err := cloudConfig.CreateLoadBalancer(
			context.TODO(),
			azkubeLoadBalancerName,
			publicIPName); err != nil {
			return reconcile.Result{}, err
		}
		log.Info("Successfully Created Public Load Balancer", "Name", azkubeLoadBalancerName, "PublicIPName", publicIPName)

		pip, err := cloudConfig.GetPublicIP(context.TODO(), publicIPName)
		if err != nil {
			return reconcile.Result{}, err
		}
		publicAddress = *pip.PublicIPAddressPropertiesFormat.IPAddress
		log.Info("Successfully Established Public IP", "Name", publicIPName, "Address", publicAddress)
	}

	defer os.RemoveAll(tmpDir + instance.Name)

	v1beta1cfg := &kubeadmv1beta1.InitConfiguration{}
	kubeadmscheme.Scheme.Default(v1beta1cfg)
	v1beta1cfg.CertificatesDir = tmpDir + request.Name + "/certs"
	v1beta1cfg.Etcd.Local = &kubeadmv1beta1.LocalEtcd{}
	v1beta1cfg.LocalAPIEndpoint = kubeadmv1beta1.APIEndpoint{AdvertiseAddress: "192.0.0.4", BindPort: 6443}
	v1beta1cfg.ControlPlaneEndpoint = fmt.Sprintf("%s:6443", internalDNSName)
	if publicAddress != "" {
		v1beta1cfg.APIServer.CertSANs = []string{"192.0.0.100", publicDNSName, internalDNSName}
	}
	v1beta1cfg.NodeRegistration.Name = "fakenode" + request.Name
	cfg := &kubeadmapi.InitConfiguration{}
	kubeadmscheme.Scheme.Default(cfg)
	kubeadmscheme.Scheme.Convert(v1beta1cfg, cfg, nil)

	log.Info("Creating PKI Certificates", "InternalDNS", internalDNSName)
	if err := createCertificates(cfg); err != nil {
		log.Error(err, "Error Generating Certificates")
		return reconcile.Result{RequeueAfter: 3 * time.Second}, err
	}
	log.Info("Successfully Created PKI Certificates", "InternalDNS", internalDNSName)

	log.Info("Creating Kubeconfigs")
	kubeConfigDir := tmpDir + request.Name + "/kubeconfigs"
	if err := createKubeconfigs(cfg, kubeConfigDir); err != nil {
		log.Error(err, "Error Generating Kubeconfigs")
		return reconcile.Result{RequeueAfter: 3 * time.Second}, err
	}
	log.Info("Successfully Created Kubeconfigs")

	if err := updateStatus(instance); err != nil {
		log.Error(err, "Error Updating Status")
		return reconcile.Result{RequeueAfter: 3 * time.Second}, err
	}

	if publicAddress != "" {
		log.Info("Creating Customer Kubeconfig", "DNS", publicDNSName)
		os.Remove(tmpDir + instance.Name + "/kubeconfigs/admin.conf")
		cfg.LocalAPIEndpoint = kubeadmapi.APIEndpoint{AdvertiseAddress: "192.0.0.4", BindPort: 6443}
		cfg.ControlPlaneEndpoint = fmt.Sprintf("%s:443", publicDNSName)
		if err := kubeconfigphase.CreateKubeConfigFile(kubeadmconstants.AdminKubeConfigFileName, kubeConfigDir, cfg); err != nil {
			return reconcile.Result{RequeueAfter: 3 * time.Second}, err
		}
		buf, err := ioutil.ReadFile(tmpDir + instance.Name + "/kubeconfigs/admin.conf")
		if err != nil {
			return reconcile.Result{RequeueAfter: 3 * time.Second}, err
		}
		instance.Status.CustomerKubeConfig = string(buf)
		instance.Status.PublicDNSName = publicDNSName
		log.Info("Created Customer Kubeconfig", "DNS", publicDNSName)
	}

	instance.Status.InternalDNSName = internalDNSName
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

	token, err := bootstraputil.GenerateBootstrapToken()
	if err != nil {
		return err
	}
	instance.Status.BootstrapToken = token

	discoveryHashes, err := getDiscoveryHashes(tmpDir + instance.Name + "/kubeconfigs/admin.conf")
	if err != nil {
		return err
	}
	instance.Status.DiscoveryHashes = discoveryHashes

	cloudConfig := getCloudConfig(instance)
	instance.Status.CloudConfig = cloudConfig
	return nil
}

func getDiscoveryHashes(kubeConfigFile string) ([]string, error) {
	// load the kubeconfig file to get the CA certificate and endpoint
	config, err := clientcmd.LoadFromFile(kubeConfigFile)
	if err != nil {
		return nil, err
	}

	// load the default cluster config
	clusterConfig := kubeconfigutil.GetClusterFromKubeConfig(config)
	if clusterConfig == nil {
		return nil, fmt.Errorf("failed to get default cluster config")
	}

	// load CA certificates from the kubeconfig (either from PEM data or by file path)
	var caCerts []*x509.Certificate
	if clusterConfig.CertificateAuthorityData != nil {
		caCerts, err = clientcertutil.ParseCertsPEM(clusterConfig.CertificateAuthorityData)
		if err != nil {
			return nil, err
		}
	} else if clusterConfig.CertificateAuthority != "" {
		caCerts, err = clientcertutil.CertsFromFile(clusterConfig.CertificateAuthority)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("no CA certificates found in kubeconfig")
	}

	// hash all the CA certs and include their public key pins as trusted values
	publicKeyPins := make([]string, 0, len(caCerts))
	for _, caCert := range caCerts {
		publicKeyPins = append(publicKeyPins, pubkeypin.Hash(caCert))
	}
	return publicKeyPins, nil
}

func getCloudConfig(instance *enginev1alpha1.Cluster) string {
	return fmt.Sprintf(`{
"cloud":"AzurePublicCloud",
"tenantId": "%[1]s",
"subscriptionId": "%[2]s",
"aadClientId": "%[3]s",
"aadClientSecret": "%[4]s",
"resourceGroup": "%[5]s",
"location": "%[6]s",
"vmType": "vmss",
"subnetName": "agent-subnet",
"securityGroupName": "azkube-nsg",
"vnetName": "azkube-vnet",
"vnetResourceGroup": "%[5]s",
"routeTableName": "azkube-routetable",
"primaryAvailabilitySetName": "",
"primaryScaleSetName": "",
"cloudProviderBackoff": true,
"cloudProviderBackoffRetries": 6,
"cloudProviderBackoffExponent": 1.5,
"cloudProviderBackoffDuration": 5,
"cloudProviderBackoffJitter": 1.0,
"cloudProviderRatelimit": true,
"cloudProviderRateLimitQPS": 3.0,
"cloudProviderRateLimitBucket": 10,
"useManagedIdentityExtension": false,
"userAssignedIdentityID": "",
"useInstanceMetadata": true,
"loadBalancerSku": "Standard",
"excludeMasterFromStandardLB": true,
"providerVaultName": "",
"maximumLoadBalancerRuleCount": 250,
"providerKeyName": "k8s",
"providerKeyVersion": ""
}`,
		instance.Spec.TenantID,
		instance.Spec.SubscriptionID,
		instance.Spec.ClientID,
		instance.Spec.ClientSecret,
		instance.Spec.ResourceGroupName,
		instance.Spec.Location,
	)
}
