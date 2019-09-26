package controllers

import (
	"context"
	"fmt"
	"hash/fnv"
	"reflect"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	enginev1alpha1 "github.com/awesomenix/azk/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodePoolReconciler reconciles a NodePool object
type NodePoolReconciler struct {
	client.Client
	Log logr.Logger
	record.EventRecorder
}

// +kubebuilder:rbac:groups=engine.azk.io,resources=nodepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=engine.azk.io,resources=nodepools/status,verbs=get;update;patch

func (r *NodePoolReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("nodepool", req.NamespacedName)

	// Fetch the NodePool instance
	instance := &enginev1alpha1.NodePool{}
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

	h := fnv.New64a()
	h.Write([]byte(fmt.Sprintf("%s/%s", instance.Name, instance.Spec.KubernetesVersion)))
	nodeSetName := fmt.Sprintf("%x", h.Sum64())
	nodeSetName = instance.Name + "-" + nodeSetName

	nodeSet := &enginev1alpha1.NodeSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeSetName,
			Namespace: instance.Namespace,
		},
		Spec: enginev1alpha1.NodeSetSpec{
			KubernetesVersion: instance.Spec.KubernetesVersion,
			Replicas:          instance.Spec.Replicas,
			VMSKUType:         instance.Spec.VMSKUType,
		},
	}
	if err := controllerutil.SetControllerReference(instance, nodeSet, nil); err != nil {
		return ctrl.Result{}, err
	}

	foundNodeSet := &enginev1alpha1.NodeSet{}
	err = r.Get(ctx, types.NamespacedName{Name: nodeSet.Name, Namespace: nodeSet.Namespace}, foundNodeSet)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating NodeSet", "namespace", nodeSet.Namespace, "name", nodeSet.Name)
		err = r.Create(ctx, nodeSet)
		if err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Successfully Created NodeSet", "NodeSet", nodeSet.Name, "Namespace", nodeSet.Namespace)
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	} else {
		if !reflect.DeepEqual(nodeSet.Spec, foundNodeSet.Spec) {
			foundNodeSet.Spec = nodeSet.Spec
			log.Info("Updating NodeSet", "namespace", nodeSet.Namespace, "name", nodeSet.Name)
			err = r.Update(ctx, foundNodeSet)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if instance.Spec.Replicas != nil && int32(len(foundNodeSet.Status.NodeStatus)) == *instance.Spec.Replicas {
		if err := r.performGarbageCollection(instance.Namespace, nodeSetName); err != nil {
			return ctrl.Result{}, err
		}
	}

	instance.Status.NodeSetName = nodeSetName
	instance.Status.Replicas = foundNodeSet.Status.Replicas
	instance.Status.VMReplicas = int32(len(foundNodeSet.Status.NodeStatus))
	instance.Status.KubernetesVersion = foundNodeSet.Status.KubernetesVersion
	instance.Status.ProvisioningState = foundNodeSet.Status.ProvisioningState
	if err := r.Status().Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NodePoolReconciler) performGarbageCollection(namespace, nodeSetName string) error {
	log := r.Log.WithValues("nodepool", nodeSetName)

	nodeSetList := enginev1alpha1.NodeSetList{}
	if err := r.List(context.TODO(), &nodeSetList, client.InNamespace(namespace)); err != nil {
		return err
	}

	for _, nodeSet := range nodeSetList.Items {
		if nodeSet.Name == nodeSetName {
			continue
		}

		log.Info("Deleting Unreferenced NodeSet", "namespace", nodeSet.Namespace, "name", nodeSet.Name)
		if err := r.Delete(context.TODO(), &nodeSet); err != nil {
			return err
		}
	}

	return nil
}

func (r *NodePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&enginev1alpha1.NodePool{}).
		Complete(r)
}
