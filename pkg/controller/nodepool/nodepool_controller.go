package nodepool

import (
	"context"
	"fmt"
	"hash/fnv"
	"reflect"

	enginev1alpha1 "github.com/awesomenix/azkube/pkg/apis/engine/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller")

// Add creates a new NodePool Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNodePool{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("nodepool-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to NodePool
	err = c.Watch(&source.Kind{Type: &enginev1alpha1.NodePool{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &enginev1alpha1.NodeSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &enginev1alpha1.NodePool{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileNodePool{}

// ReconcileNodePool reconciles a NodePool object
type ReconcileNodePool struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a NodePool object and makes changes based on the state read
// and what is in the NodePool.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=engine.azkube.io,resources=nodepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=engine.azkube.io,resources=nodepools/status,verbs=get;update;patch
func (r *ReconcileNodePool) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the NodePool instance
	instance := &enginev1alpha1.NodePool{}
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

	h := fnv.New64a()
	h.Write([]byte(fmt.Sprintf("%s/%s", instance.Name, instance.Spec.KubernetesVersion)))
	nodeSetName := fmt.Sprintf("%x", h.Sum64())

	nodeSet := &enginev1alpha1.NodeSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-" + nodeSetName,
			Namespace: instance.Namespace,
		},
		Spec: enginev1alpha1.NodeSetSpec{
			KubernetesVersion: instance.Spec.KubernetesVersion,
			Replicas:          instance.Spec.Replicas,
		},
	}
	if err := controllerutil.SetControllerReference(instance, nodeSet, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	foundNodeSet := &enginev1alpha1.NodeSet{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: nodeSet.Name, Namespace: nodeSet.Namespace}, foundNodeSet)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating NodeSet", "namespace", nodeSet.Namespace, "name", nodeSet.Name)
		err = r.Create(context.TODO(), nodeSet)
		if err != nil {
			return reconcile.Result{}, err
		}
		log.Info("Successfully Created NodeSet", "NodeSet", nodeSet.Name, "Namespace", nodeSet.Namespace)
	} else if err != nil {
		return reconcile.Result{}, err
	} else {
		if !reflect.DeepEqual(nodeSet.Spec, foundNodeSet.Spec) {
			foundNodeSet.Spec = nodeSet.Spec
			log.Info("Updating NodeSet", "namespace", nodeSet.Namespace, "name", nodeSet.Name)
			err = r.Update(context.TODO(), foundNodeSet)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	instance.Status.NodeSetName = nodeSetName
	//instance.Status.PrevNodeSetName = nodeSetName
	instance.Status.Replicas = foundNodeSet.Status.Replicas
	instance.Status.VMReplicas = int32(len(foundNodeSet.Status.NodeStatus))
	instance.Status.KubernetesVersion = foundNodeSet.Status.KubernetesVersion
	instance.Status.ProvisioningState = foundNodeSet.Status.ProvisioningState
	if err := r.Status().Update(context.TODO(), instance); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
