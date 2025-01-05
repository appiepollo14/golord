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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	avastennlv1alpha1 "github.com/appiepollo14/golord/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FetchReconciler reconciles a Fetch object
type FetchReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=avasten.nl,resources=fetches,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=avasten.nl,resources=fetches/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=avasten.nl,resources=fetches/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Fetch object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *FetchReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Fetch instance
	var fetch avastennlv1alpha1.Fetch
	if err := r.Get(ctx, req.NamespacedName, &fetch); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Fetch resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Fetch")
		return ctrl.Result{}, err
	}

	// Inside the Reconcile function of FetchReconciler...
	// Fetch the Gofer instance based on the Gofername in Fetch.Spec.Gofername
	var gofer avastennlv1alpha1.Gofer
	if err := r.Get(ctx, types.NamespacedName{Name: fetch.Spec.Gofername}, &gofer); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Gofer resource not found, cannot set owner reference for Deployment")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Gofer")
		return ctrl.Result{}, err
	}

	// Step 1: Ensure the Namespace exists
	namespaceName := "default"
	var namespace corev1.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: namespaceName}, &namespace); err != nil {
		if errors.IsNotFound(err) {
			// Namespace does not exist, so create it
			namespace = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespaceName,
				},
			}
			if err := r.Create(ctx, &namespace); err != nil {
				log.Error(err, "Failed to create Namespace", "Namespace.Name", namespaceName)
				return ctrl.Result{}, err
			}
			log.Info("Namespace created successfully", "Namespace.Name", namespaceName)
		} else {
			log.Error(err, "Failed to get Namespace")
			return ctrl.Result{}, err
		}
	}

	// Step 2: Ensure the Deployment exists in the Namespace
	deploymentName := fmt.Sprintf("%s-nginx-deployment", fetch.Name)
	var deployment appsv1.Deployment
	// Deployment does not exist, so create it
	deployment = appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespaceName,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &fetch.Spec.Deployments,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "nginx"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "nginx"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx",
						},
					},
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(&fetch, &deployment, r.Scheme); err != nil {
		log.Error(err, "Failed to set owner reference on Deployment")
		return ctrl.Result{}, err
	}
	if err := r.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: namespaceName}, &deployment); err != nil {
		if errors.IsNotFound(err) {
			// Set Gofer as owner of Deployment using SetControllerReference

			if err := r.Create(ctx, &deployment); err != nil {
				log.Error(err, "Failed to create Deployment", "Deployment.Name", deploymentName)
				return ctrl.Result{}, err
			}
			log.Info("Deployment created successfully", "Deployment.Name", deploymentName)
		} else {
			log.Error(err, "Failed to get Deployment")
			return ctrl.Result{}, err
		}
	} else {
		log.Info("Deployment exists")
		if deployment.Spec.Replicas != &fetch.Spec.Deployments {
			deployment.Spec.Replicas = &fetch.Spec.Deployments
			if err = r.Update(ctx, &deployment); err != nil {
				log.Error(err, "Update deployment failed")
				return ctrl.Result{}, err
			}
			log.Info("Update deployment succeeeded")
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FetchReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&avastennlv1alpha1.Fetch{}).
		Owns(&appsv1.Deployment{}).
		Watches(&avastennlv1alpha1.Gofer{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, gofer client.Object) []ctrl.Request {
			// map a change from referenced configMap to ExampleCRDWithConfigMapRef, which causes its re-reconcile
			fetches := &avastennlv1alpha1.FetchList{}
			if err := mgr.GetClient().List(ctx, fetches); err != nil {
				mgr.GetLogger().Error(err, "while listing fetches")
				return nil
			}

			reqs := make([]ctrl.Request, 0, len(fetches.Items))
			for _, item := range fetches.Items {
				if item.Spec.Gofername == gofer.GetName() {
					mgr.GetLogger().Info("Name equals")
					reqs = append(reqs, ctrl.Request{
						NamespacedName: types.NamespacedName{
							Namespace: item.GetNamespace(),
							Name:      item.GetName(),
						},
					})
				}
			}

			return reqs
		})).
		Complete(r)
}
