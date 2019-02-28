package nodeset

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
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

const (
	nodesetsFinalizerName = "nodesets.finalizers.engine.azkube.io"
)

var log = logf.Log.WithName("controller")

func getEncodedNodeSetStartupScript(bootstrapToken, discoveryHash string) string {
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
sudo kubeadm join 192.0.0.4:6443 --token %[1]s --discovery-token-ca-cert-hash %[2]s
`, bootstrapToken, discoveryHash)))
	return startupScript
}

// Add creates a new NodeSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNodeSet{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("nodeset-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to NodeSet
	err = c.Watch(&source.Kind{Type: &enginev1alpha1.NodeSet{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileNodeSet{}

// ReconcileNodeSet reconciles a NodeSet object
type ReconcileNodeSet struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a NodeSet object and makes changes based on the state read
// and what is in the NodeSet.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=engine.azkube.io,resources=nodesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=engine.azkube.io,resources=nodesets/status,verbs=get;update;patch
func (r *ReconcileNodeSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the NodeSet instance
	instance := &enginev1alpha1.NodeSet{}
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

	cluster, err := r.getCluster(context.TODO(), instance.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	if cluster.Status.ProvisioningState != "Succeeded" {
		// Wait for cluster to initialize
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
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

	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object.
		update := false

		if !helpers.ContainsFinalizer(instance.ObjectMeta.Finalizers, nodesetsFinalizerName) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, nodesetsFinalizerName)
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
		if helpers.ContainsFinalizer(instance.ObjectMeta.Finalizers, nodesetsFinalizerName) {
			if cloudConfig.IsValid() {
				// our finalizer is present, so lets handle our external dependency
				if err := deleteNodeSet(context.TODO(), instance, cloudConfig); err != nil {
					// if fail to delete the external dependency here, return with error
					// so that it can be retried
					// meh! its fine if it fails, we definitely need to wait here for it to be deleted
				}
			}

			// remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = helpers.RemoveFinalizer(instance.ObjectMeta.Finalizers, nodesetsFinalizerName)
			if err := r.Update(context.TODO(), instance); err != nil {
				return reconcile.Result{Requeue: true}, nil
			}
		}

		return reconcile.Result{}, nil
	}

	customData := map[string]string{
		"/tmp/ca.crt": cluster.Status.CACertificate,
		"/tmp/ca.key": cluster.Status.CACertificateKey,
	}

	if err := updateNodeSet(instance, cloudConfig); err != nil {
		instance.Status.ProvisioningState = "Updating"
		if err := r.Status().Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}

		if err := cloudConfig.CreateVMSS(
			context.TODO(),
			instance.Name+"-agentvmss",
			"azkube-vnet",
			"agent-subnet",
			getEncodedNodeSetStartupScript(cluster.Status.BootstrapToken, cluster.Status.DiscoveryHashes[0]),
			azhelpers.GetCustomData(customData),
		); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	} else if int(*instance.Spec.Replicas) != len(instance.Status.NodeStatus) {
		instance.Status.ProvisioningState = "Scaling"
		if err := r.Status().Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}

		if err := scaleNodeSet(context.TODO(), instance, cloudConfig); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	instance.Status.KubernetesVersion = instance.Spec.KubernetesVersion
	instance.Status.ProvisioningState = "Succeeded"
	instance.Status.Kubeconfig = cluster.Status.CustomerKubeConfig
	instance.Status.Replicas = int32(len(instance.Status.NodeStatus))
	if err := r.Status().Update(context.TODO(), instance); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileNodeSet) getCluster(ctx context.Context, namespace string) (*enginev1alpha1.Cluster, error) {
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

func deleteNodeSet(ctx context.Context, instance *enginev1alpha1.NodeSet, cloudConfig azhelpers.CloudConfiguration) error {
	vmssName := instance.Name + "-agentvmss"

	for _, vms := range instance.Status.NodeStatus {
		err := cordonDrainAndDeleteNode(instance.Status.Kubeconfig, vms.VMComputerName)
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

func updateNodeSet(instance *enginev1alpha1.NodeSet, cloudConfig azhelpers.CloudConfiguration) error {
	vmssName := instance.Name + "-agentvmss"
	vmssClient, err := cloudConfig.GetVMSSVMsClient()
	if err != nil {
		log.Error(err, "Error GetVMSSVMsClient", "VMSS", vmssName)
		return err
	}

	result, err := vmssClient.List(context.TODO(), cloudConfig.GroupName, vmssName, "", "", string(compute.InstanceView))
	if err != nil {
		log.Error(err, "Error VMSSClient List", "VMSS", vmssName)
		return err
	}

	var nodesetVMStatus []enginev1alpha1.NodeSetVMStatus
	for _, vmID := range result.Values() {
		log.Info("Appending to VMSS Nodepool list", "VM", *vmID.OsProfile.ComputerName)
		var status enginev1alpha1.NodeSetVMStatus
		status.VMComputerName = *vmID.OsProfile.ComputerName
		status.VMInstanceID = *vmID.InstanceID
		nodesetVMStatus = append(nodesetVMStatus, status)
	}
	instance.Status.NodeStatus = nodesetVMStatus
	return nil
}

func scaleNodeSet(ctx context.Context, instance *enginev1alpha1.NodeSet, cloudConfig azhelpers.CloudConfiguration) error {
	vmssName := instance.Name + "-agentvmss"
	expectedCount := int(*instance.Spec.Replicas)
	if len(instance.Status.NodeStatus) < expectedCount {
		curCount := 0
		vmssName := instance.Name + "-agentvmss"
		for _, nodeStatus := range instance.Status.NodeStatus {
			if curCount < expectedCount {
				curCount++
				continue
			}

			err := cordonDrainAndDeleteNode(instance.Status.Kubeconfig, nodeStatus.VMComputerName)
			if err != nil {
				return err
			}

			vmssClient, err := cloudConfig.GetVMSSVMsClient()
			if err != nil {
				return err
			}

			log.Info("Scaling down", "VMSS", nodeStatus.VMInstanceID)

			_, err = vmssClient.Delete(ctx, cloudConfig.GroupName, vmssName, nodeStatus.VMInstanceID)
			if err != nil {
				return err
			}
		}
	}
	return cloudConfig.ScaleVMSS(ctx, vmssName, expectedCount)
}
