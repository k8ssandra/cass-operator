/*
Copyright 2021.

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

package controllers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/k8ssandra/cass-operator/pkg/dynamicwatch"
	"github.com/k8ssandra/cass-operator/pkg/oplabels"
	"github.com/k8ssandra/cass-operator/pkg/reconciliation"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	api "github.com/k8ssandra/cass-operator/api/v1beta1"
)

// datastax.com groups
//+kubebuilder:rbac:groups=cassandra.datastax.com,namespace=cass-operator,resources=cassandradatacenters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cassandra.datastax.com,namespace=cass-operator,resources=cassandradatacenters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cassandra.datastax.com,namespace=cass-operator,resources=cassandradatacenters/finalizers,verbs=update;delete

// Kubernetes core
// +kubebuilder:rbac:groups=apps,namespace=cass-operator,resources=statefulsets;replicasets;deployments;daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,namespace=cass-operator,resources=deployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,namespace=cass-operator,resources=pods;endpoints;services;configmaps;secrets;persistentvolumeclaims;events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,namespace=cass-operator,resources=namespaces,verbs=get
// +kubebuilder:rbac:groups=core,resources=persistentvolumes;nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=policy,namespace=cass-operator,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete

// CassandraDatacenterReconciler reconciles a cassandraDatacenter object
type CassandraDatacenterReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// SecretWatches is used in the controller when setting up the watches and
	// during reconciliation where we update the mappings for the watches.
	// Putting it here allows us to get it to both places.
	SecretWatches dynamicwatch.DynamicWatches
}

// Reconcile reads that state of the cluster for a Datacenter object
// and makes changes based on the state read
// and what is in the cassandraDatacenter.Spec
// Note:
// The Controller will requeue the Request to be processed again
// if the returned error is non-nil or Result.Requeue is true,
// otherwise upon completion it will remove the work from the queue.
// See: https://godoc.org/sigs.k8s.io/controller-runtime/pkg/reconcile#Result
func (r *CassandraDatacenterReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	startReconcile := time.Now()

	logger := r.Log.
		WithValues("cassandradatacenter", request.NamespacedName).
		WithValues("requestNamespace", request.Namespace).
		WithValues("requestName", request.Name).
		// loopID is used to tie all events together that are spawned by the same reconciliation loop
		WithValues("loopID", uuid.New().String())

	defer func() {
		reconcileDuration := time.Since(startReconcile).Seconds()
		logger.Info("Reconcile loop completed",
			"duration", reconcileDuration)
	}()

	logger.Info("======== handler::Reconcile has been called")

	rc, err := reconciliation.CreateReconciliationContext(&request, r.Client, r.Scheme, r.Recorder, r.SecretWatches, logger)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// Return and don't requeue
			logger.Info("CassandraDatacenter resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}

		// Error reading the object
		logger.Error(err, "Failed to get CassandraDatacenter.")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}

	if err := rc.IsValid(rc.Datacenter); err != nil {
		logger.Error(err, "CassandraDatacenter resource is invalid")
		rc.Recorder.Eventf(rc.Datacenter, "Warning", "ValidationFailed", err.Error())
		return ctrl.Result{}, err
	}

	// TODO fold this into the quiet period
	twentySecs := time.Second * 20
	lastNodeStart := rc.Datacenter.Status.LastServerNodeStarted
	cooldownTime := time.Until(lastNodeStart.Add(twentySecs))

	if cooldownTime > 0 {
		logger.Info("Ending reconciliation early because a server node was recently started")
		secs := time.Duration(1 + int(cooldownTime.Seconds()))
		return ctrl.Result{RequeueAfter: secs * time.Second}, nil
	}

	if rc.Datacenter.Status.QuietPeriod.After(time.Now()) {
		logger.Info("Ending reconciliation early because the datacenter is in a quiet period")
		cooldownTime = rc.Datacenter.Status.QuietPeriod.Sub(time.Now())
		secs := time.Duration(1 + int(cooldownTime.Seconds()))
		return ctrl.Result{RequeueAfter: secs * time.Second}, nil
	}

	res, err := rc.CalculateReconciliationActions()
	if err != nil {
		logger.Error(err, "calculateReconciliationActions returned an error")
		rc.Recorder.Eventf(rc.Datacenter, "Warning", "ReconcileFailed", err.Error())
	}
	return res, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *CassandraDatacenterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	log := r.Log.
		WithName("cassandradatacenter_controller")

	managedByCassandraOperatorPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return oplabels.HasManagedByCassandraOperatorLabel(e.Object.GetLabels())
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return oplabels.HasManagedByCassandraOperatorLabel(e.Object.GetLabels())
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return oplabels.HasManagedByCassandraOperatorLabel(e.ObjectOld.GetLabels()) ||
				oplabels.HasManagedByCassandraOperatorLabel(e.ObjectNew.GetLabels())
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return oplabels.HasManagedByCassandraOperatorLabel(e.Object.GetLabels())
		},
	}

	// Create a new managed controller builder
	c := ctrl.NewControllerManagedBy(mgr).
		Named("cassandradatacenter-controller").
		WithLogger(log).
		For(&api.CassandraDatacenter{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&appsv1.StatefulSet{}, builder.WithPredicates(managedByCassandraOperatorPredicate)).
		Owns(&policyv1beta1.PodDisruptionBudget{}, builder.WithPredicates(managedByCassandraOperatorPredicate)).
		Owns(&corev1.Service{}, builder.WithPredicates(managedByCassandraOperatorPredicate))

	configSecretMapFn := func(mapObj client.Object) []reconcile.Request {
		log.Info("config secret watch called", "Secret", mapObj.GetName())

		requests := make([]reconcile.Request, 0)
		secret := mapObj.(*corev1.Secret)
		if v, ok := secret.Annotations[api.DatacenterAnnotation]; ok {
			log.Info("adding reconciliation request for config secret", "Secret", secret.Name)
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: secret.Namespace,
					Name:      v,
				},
			})
		}

		return requests
	}

	isConfigSecret := func(annotations map[string]string) bool {
		_, ok := annotations[api.DatacenterAnnotation]
		return ok
	}

	configSecretPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return isConfigSecret(e.Object.GetAnnotations())
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			return isConfigSecret(e.ObjectOld.GetAnnotations()) || isConfigSecret(e.ObjectNew.GetAnnotations())
		},

		DeleteFunc: func(e event.DeleteEvent) bool {
			return isConfigSecret(e.Object.GetAnnotations())
		},

		GenericFunc: func(e event.GenericEvent) bool {
			return isConfigSecret(e.Object.GetAnnotations())
		},
	}

	c = c.Watches(&source.Kind{Type: &corev1.Secret{}}, handler.EnqueueRequestsFromMapFunc(configSecretMapFn), builder.WithPredicates(configSecretPredicate))

	// TODO Add PSP stuff here if necessary

	// Setup watches for Secrets. These secrets are often not owned by or created by
	// the operator, so we must create a mapping back to the appropriate datacenters.

	r.SecretWatches = dynamicwatch.NewDynamicSecretWatches(r.Client)
	dynamicSecretWatches := r.SecretWatches

	toRequests := func(a client.Object) []reconcile.Request {
		watchers := dynamicSecretWatches.FindWatchers(a)
		requests := []reconcile.Request{}
		for _, watcher := range watchers {
			requests = append(requests, reconcile.Request{NamespacedName: watcher})
		}
		return requests
	}

	c = c.Watches(
		&source.Kind{Type: &corev1.Secret{}},
		handler.EnqueueRequestsFromMapFunc(toRequests),
	)

	return c.Complete(r)
}

// blank assignment to verify that CassandraDatacenterReconciler implements reconciliation.Reconciler
var _ reconcile.Reconciler = &CassandraDatacenterReconciler{}
