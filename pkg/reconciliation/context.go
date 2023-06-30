// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/dynamicwatch"
	"github.com/k8ssandra/cass-operator/pkg/events"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
)

// ReconciliationContext contains all of the input necessary to calculate a list of ReconciliationActions
type ReconciliationContext struct {
	Request        *reconcile.Request
	Client         client.Client
	Scheme         *runtime.Scheme
	Datacenter     *api.CassandraDatacenter
	NodeMgmtClient httphelper.NodeMgmtClient
	Recorder       record.EventRecorder
	ReqLogger      logr.Logger
	SecretWatches  dynamicwatch.DynamicWatches
	OpenShift      bool

	// According to golang recommendations the context should not be stored in a struct but given that
	// this is passed around as a parameter we feel that its a fair compromise. For further discussion
	// see: golang/go#22602
	Ctx context.Context

	Services               []*corev1.Service
	Endpoints              *corev1.Endpoints
	desiredRackInformation []*RackInformation
	statefulSets           []*appsv1.StatefulSet
	dcPods                 []*corev1.Pod
	clusterPods            []*corev1.Pod
}

// CreateReconciliationContext gathers all information needed for computeReconciliationActions into a struct.
func CreateReconciliationContext(
	ctx context.Context,
	req *reconcile.Request,
	cli client.Client,
	scheme *runtime.Scheme,
	rec record.EventRecorder,
	secretWatches dynamicwatch.DynamicWatches,
	runningInOpenshift bool) (*ReconciliationContext, error) {

	reqLogger := log.FromContext(ctx)
	rc := &ReconciliationContext{}
	rc.Request = req
	rc.Client = cli
	rc.Scheme = scheme
	rc.Recorder = &events.LoggingEventRecorder{EventRecorder: rec, ReqLogger: reqLogger}
	rc.SecretWatches = secretWatches
	rc.ReqLogger = reqLogger
	rc.Ctx = ctx
	rc.OpenShift = runningInOpenshift

	rc.ReqLogger = rc.ReqLogger.
		WithValues("namespace", req.Namespace)

	rc.ReqLogger.Info("handler::CreateReconciliationContext")

	// Fetch the datacenter resource
	dc := &api.CassandraDatacenter{}
	if err := retrieveDatacenter(rc, req, dc); err != nil {
		rc.ReqLogger.Error(err, "error in retrieveDatacenter")
		return nil, err
	}
	rc.Datacenter = dc

	// workaround for kubernetes having problems with zero-value and nil Times
	if rc.Datacenter.Status.SuperUserUpserted.IsZero() {
		rc.Datacenter.Status.SuperUserUpserted = metav1.Unix(1, 0)
	}
	if rc.Datacenter.Status.LastServerNodeStarted.IsZero() {
		rc.Datacenter.Status.LastServerNodeStarted = metav1.Unix(1, 0)
	}
	if rc.Datacenter.Status.LastRollingRestart.IsZero() {
		rc.Datacenter.Status.LastRollingRestart = metav1.Unix(1, 0)
	}

	rc.ReqLogger = rc.ReqLogger.
		WithValues("datacenterName", dc.SanitizedName()).
		WithValues("clusterName", dc.Spec.ClusterName)

	log.IntoContext(ctx, rc.ReqLogger)

	var err error
	rc.NodeMgmtClient, err = httphelper.NewMgmtClient(rc.Ctx, cli, dc)
	if err != nil {
		rc.ReqLogger.Error(err, "failed to build NodeMgmtClient")
		return nil, err
	}

	return rc, nil
}

func retrieveDatacenter(rc *ReconciliationContext, request *reconcile.Request, dc *api.CassandraDatacenter) error {
	err := rc.Client.Get(
		rc.Ctx,
		request.NamespacedName,
		dc)
	return err
}

func (rc *ReconciliationContext) GetLogger() logr.Logger {
	return rc.ReqLogger
}

func (rc *ReconciliationContext) GetClient() client.Client {
	return rc.Client
}

func (rc *ReconciliationContext) GetDatacenter() *api.CassandraDatacenter {
	return rc.Datacenter
}

func (rc *ReconciliationContext) SetDatacenterAsOwner(controlled metav1.Object) error {
	return setControllerReference(rc.Datacenter, controlled, rc.Scheme)
}

func (rc *ReconciliationContext) GetContext() context.Context {
	return rc.Ctx
}

func (rc *ReconciliationContext) validateDatacenterNameConflicts() []error {
	dc := rc.Datacenter
	var errs []error

	// Check if our CassandraDatacenter sanitized name conflicts with another CassandraDatacenter in the same namespace
	cassandraDatacenters := &api.CassandraDatacenterList{}
	err := rc.Client.List(rc.Ctx, cassandraDatacenters, &client.ListOptions{Namespace: dc.Namespace})
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to list CassandraDatacenters in namespace %s: %w", dc.Namespace, err))
	} else {
		for _, existingDc := range cassandraDatacenters.Items {
			if existingDc.SanitizedName() == dc.SanitizedName() && existingDc.Name != dc.Name {
				errs = append(errs, fmt.Errorf("datacenter name/override %s/%s is already in use by CassandraDatacenter %s/%s", dc.Name, dc.SanitizedName(), existingDc.Name, existingDc.SanitizedName()))
			}
		}
	}

	return errs
}

func (rc *ReconciliationContext) validateDatacenterNameOverride() []error {
	dc := rc.Datacenter
	var errs []error

	if dc.Status.DatacenterName == nil {
		// We didn't have a full reconcile yet, we can proceed with the override
		return errs
	} else {
		if *dc.Status.DatacenterName != dc.Spec.DatacenterName {
			errs = append(errs, fmt.Errorf("datacenter %s name override '%s' cannot be changed after creation to '%s'", dc.Name, dc.Spec.DatacenterName, *dc.Status.DatacenterName))
		}
	}

	return errs
}
