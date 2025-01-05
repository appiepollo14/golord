/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	avastennlv1alpha1 "github.com/appiepollo14/golord/api/v1alpha1"
	"github.com/appiepollo14/golord/monitoring"
	"github.com/prometheus/client_golang/prometheus"
)

// GoferReconciler reconciles a Gofer object
type GoferReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=avasten.nl,resources=gofers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=avasten.nl,resources=gofers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=avasten.nl,resources=gofers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Gofer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
// Reconcile is part of the main Kubernetes reconciliation loop
func (r *GoferReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reconcileDurationTimer := prometheus.NewTimer(monitoring.ReconcileDurationTimer)
	defer reconcileDurationTimer.ObserveDuration()

	log := log.FromContext(ctx)

	log.Info("Reconciling Gofer: " + req.Name)

	// Fetch the Gofer instance
	gofer := &avastennlv1alpha1.Gofer{}
	if err := r.Get(ctx, req.NamespacedName, gofer); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Gofer resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Gofer")
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status is available
	if gofer.Status.Conditions == nil || len(gofer.Status.Conditions) == 0 {
		meta.SetStatusCondition(&gofer.Status.Conditions, metav1.Condition{Type: avastennlv1alpha1.TypeReconciledGofer, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err := r.Status().Update(ctx, gofer); err != nil {
			log.Error(err, "Failed to update Gofer status")
			return ctrl.Result{}, err
		}

		// Let's re-fetch the memcached Custom Resource after updating the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raising the error "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, gofer); err != nil {
			log.Error(err, "Failed to re-fetch Gofer")
			return ctrl.Result{}, err
		}
	}

	// Check if the "default" namespace exists
	var namespace corev1.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: "default"}, &namespace); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Namespace 'default' not found, cannot create Fetch resource")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get namespace 'default'")
		return ctrl.Result{}, err
	}

	// Create the Fetch CR in the "default" namespace
	fetch := &avastennlv1alpha1.Fetch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-fetch", gofer.Name),
			Namespace: "default",
		},
		Spec: avastennlv1alpha1.FetchSpec{
			Deployments: gofer.Spec.Deployments,
			Gofername:   gofer.Name,
		},
	}

	// Check if Fetch CR already exists
	var existingFetch avastennlv1alpha1.Fetch
	err := r.Get(ctx, types.NamespacedName{Name: fetch.Name, Namespace: fetch.Namespace}, &existingFetch)
	if err != nil && errors.IsNotFound(err) {
		// If Fetch does not exist, create it
		if err := r.Create(ctx, fetch); err != nil {
			log.Error(err, "Failed to create Fetch CR")
			return ctrl.Result{}, err
		}
		log.Info("Fetch CR created successfully", "Fetch.Name", fetch.Name)
	} else if err != nil {
		// Error occurred in getting Fetch
		log.Error(err, "Failed to get existing Fetch CR")
		return ctrl.Result{}, err
	}

	log.Info("Resourceversion of Fetch before update: ", "version", existingFetch.ResourceVersion)
	// If there is no update to spec, the resourceversion does not change!
	if err = r.Update(ctx, &existingFetch); err != nil {
		log.Error(err, "err")
		// updating the fetch failed
		return ctrl.Result{}, err
	} else {
		log.Info("Update Fetch succeeded")
		_ = r.Get(ctx, types.NamespacedName{Name: fetch.Name, Namespace: fetch.Namespace}, &existingFetch)
		log.Info("Resourceversion of Fetch after update: ", "version", existingFetch.ResourceVersion)
	}

	// The following implementation will update the status
	meta.SetStatusCondition(&gofer.Status.Conditions, metav1.Condition{Type: avastennlv1alpha1.TypeReconciledGofer,
		Status: metav1.ConditionTrue, Reason: "Reconciling", ObservedGeneration: gofer.Generation,
		Message: fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", gofer.Name, gofer.Spec.Deployments)})

	if err := r.Status().Update(ctx, gofer); err != nil {
		log.Error(err, "Failed to update Memcached status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GoferReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&avastennlv1alpha1.Gofer{}).
		Owns(&appsv1.Deployment{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
