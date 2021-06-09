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

	"github.com/go-logr/logr"
	"github.com/k8ssandra/cass-operator/operator/pkg/dynamicwatch"
	"github.com/k8ssandra/cass-operator/operator/pkg/oplabels"
	"github.com/k8ssandra/cass-operator/operator/pkg/reconciliation"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	// "k8s.io/cri-api/pkg/errors"
	// TODO Missmatch here due to the cass-operator requirement

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	api "github.com/k8ssandra/cass-operator/api/v1beta1"
)

// datastax.com groups
//+kubebuilder:rbac:groups=cassandra.datastax.com,resources=cassandradatacenters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cassandra.datastax.com,resources=cassandradatacenters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cassandra.datastax.com,resources=cassandradatacenters/finalizers,verbs=update;delete

// Kubernetes core
// +kubebuilder:rbac:groups=apps,resources=statefulsets;replicasets;deployments;daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=pods;endpoints;services;configmaps;secrets;persistentvolumeclaims;events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete

// SetupWithManager sets up the controller with the Manager.
func (r *CassandraDatacenterReconciler) SetupWithManager(mgr ctrl.Manager) error {
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

	r.oldReconciler = reconciliation.NewReconciler(mgr)

	c := ctrl.NewControllerManagedBy(mgr).
		For(&api.CassandraDatacenter{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&policyv1beta1.PodDisruptionBudget{}).
		Owns(&corev1.Service{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		WithEventFilter(managedByCassandraOperatorPredicate)

	// TODO Still missing pspEnabled functions

	// TODO Missing Secrets watch function

	return c.Complete(r)
}

// CassandraDatacenterReconciler reconciles a cassandraDatacenter object
type CassandraDatacenterReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	oldReconciler *reconciliation.ReconcileCassandraDatacenter

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
	logger := r.Log.
		WithValues("cassandradatacenter", request.NamespacedName).
		WithValues("requestNamespace", request.Namespace).
		WithValues("requestName", request.Name)

	logger.Info("Found item, starting reconcile", "object", request.NamespacedName)

	return r.oldReconciler.Reconcile(request)
	/*
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

		rc, err := reconciliation.CreateReconciliationContext(&request, r.Client, r.Scheme, r.Recorder, r.SecretWatches, r.Log)

		if err != nil {
			if errors.IsNotFound(err) {
				// Request object not found, could have been deleted after reconcile request.
				// Owned objects are automatically garbage collected.
				// Return and don't requeue
				r.Log.Info("CassandraDatacenter resource not found. Ignoring since object must be deleted.")
				return ctrl.Result{}, nil
			}

			// Error reading the object
			logger.Error(err, "Failed to get CassandraDatacenter.")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}

		if err := rc.isValid(rc.Datacenter); err != nil {
			logger.Error(err, "CassandraDatacenter resource is invalid")
			rc.Recorder.Eventf(rc.Datacenter, "Warning", "ValidationFailed", err.Error())
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}

		// TODO fold this into the quiet period
		twentySecs := time.Second * 20
		lastNodeStart := rc.Datacenter.Status.LastServerNodeStarted
		cooldownTime := time.Until(lastNodeStart.Add(twentySecs))

		if cooldownTime > 0 {
			logger.Info("Ending reconciliation early because a server node was recently started")
			secs := 1 + int(cooldownTime.Seconds())
			return ctrl.Result{RequeueAfter: time.Duration(secs) * time.Second}, nil
		}

		if rc.Datacenter.Status.QuietPeriod.After(time.Now()) {
			logger.Info("Ending reconciliation early because the datacenter is in a quiet period")
			cooldownTime = rc.Datacenter.Status.QuietPeriod.Sub(time.Now())
			secs := 1 + int(cooldownTime.Seconds())
			return ctrl.Result{RequeueAfter: time.Duration(secs) * time.Second}, nil
		}

		res, err := rc.calculateReconciliationActions()
		if err != nil {
			logger.Error(err, "calculateReconciliationActions returned an error")
			rc.Recorder.Eventf(rc.Datacenter, "Warning", "ReconcileFailed", err.Error())
		}
		return res, err
	*/
}
