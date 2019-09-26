package controllers

import (
	"context"
	"fmt"

	"github.com/awesomenix/azk/helpers"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	enginev1alpha1 "github.com/awesomenix/azk/api/v1alpha1"
)

const (
	clusterFinalizerName = "cluster.finalizers.engine.azk.io"
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client
	Log logr.Logger
	record.EventRecorder
}

// +kubebuilder:rbac:groups=,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=engine.azk.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=engine.azk.io,resources=clusters/status,verbs=get;update;patch

func (r *ClusterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	_ = r.Log.WithValues("cluster", req.NamespacedName)

	defer helpers.Recover()
	// Fetch the Cluster instance
	instance := &enginev1alpha1.Cluster{}
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

	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object.
		if !helpers.ContainsFinalizer(instance.ObjectMeta.Finalizers, clusterFinalizerName) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, clusterFinalizerName)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{Requeue: true}, err
			}
			// Once updates object changes we need to requeue
			return ctrl.Result{Requeue: true}, nil
		}
	} else {
		if helpers.ContainsFinalizer(instance.ObjectMeta.Finalizers, clusterFinalizerName) {
			if err == nil && instance.Spec.IsValid() {
				instance.Spec.CleanupInfrastructure()
			}

			// remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = helpers.RemoveFinalizer(instance.ObjectMeta.Finalizers, clusterFinalizerName)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{Requeue: true}, nil
			}
		}

		return ctrl.Result{}, nil
	}

	// instance.Status.ProvisioningState = "Updating"
	// if err := r.Status().Update(ctx, instance); err != nil {
	// 	return ctrl.Result{}, err
	// }

	// if instance.Spec.IsValid() {
	// 	if err := instance.Spec.Bootstrap(); err != nil {
	// 		r.recorder.Event(instance, "Warning", "Error", fmt.Sprintf("Bootstrap Failed %s", err.Error()))
	// 		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	// 	}
	// }

	instance.Status.ProvisioningState = "Succeeded"
	if err := r.Status().Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(instance, "Normal", "Created", fmt.Sprintf("Completed Cluster Setup %s/%s", req.Namespace, req.Name))
	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&enginev1alpha1.Cluster{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 30}).
		Complete(r)
}
