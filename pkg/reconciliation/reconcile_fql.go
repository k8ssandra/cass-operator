package reconciliation

import (
	"errors"

	"github.com/k8ssandra/cass-operator/internal/result"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
)

// CheckFullQueryLogging sets FQL enabled or disabled. It calls the NodeMgmtClient which calls the Cassandra management API and returns a result.ReconcileResult.
func (rc *ReconciliationContext) CheckFullQueryLogging() result.ReconcileResult {
	rc.ReqLogger.Info("reconcile_racks::CheckFullQueryLogging")
	dc := rc.GetDatacenter()
	if !dc.DeploymentSupportsFQL() {
		return result.Continue()
	}

	enableFQL, err := dc.FullQueryEnabled()
	if err != nil {
		return result.Error(err)
	}

	podList, err := rc.listPods(rc.Datacenter.GetClusterLabels())
	if err != nil {
		rc.ReqLogger.Error(err, "error listing all pods in the cluster to progress full query logging reconciliation")
		return result.RequeueSoon(2)
	}
	for _, podPtr := range podList {
		features, err := rc.NodeMgmtClient.FeatureSet(podPtr)
		if err != nil {
			rc.ReqLogger.Error(err, "failed to verify featureset for FQL support")
			return result.RequeueSoon(2)
		}
		if !features.Supports(httphelper.FullQuerySupport) {
			if enableFQL {
				err := errors.New("FQL should be enabled but we cannot verify if FQL is supported by mgmt api")
				return result.Error(err)
			}
			// FQL support not available in mgmt API but user is not requesting it - continue.
			return result.Continue()
		}
		fqlEnabledForPod, err := rc.NodeMgmtClient.CallIsFullQueryLogEnabledEndpoint(podPtr)
		if err != nil {
			rc.ReqLogger.Error(err, "can't get whether query logging enabled for pod ", "podName", podPtr.Name)
			return result.RequeueSoon(2)
		}
		if fqlEnabledForPod != enableFQL {
			rc.ReqLogger.Info("Modifying full query logging on ", "podIP", podPtr.Status.PodIP, "podName", podPtr.Name, "fqlDesiredState", enableFQL)
			err := rc.NodeMgmtClient.CallSetFullQueryLog(podPtr, enableFQL)
			if err != nil {
				rc.ReqLogger.Error(err, "couldn't enable full query logging on ", "podIP", podPtr.Status.PodIP, "podName", podPtr.Name)
				return result.RequeueSoon(2)
			}
		}
	}
	return result.Continue()
}
