package reconciliation

import (
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
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

	for _, pod := range ListAllStartedPods(rc.dcPods) {
		if !isServerReady(pod) {
			continue
		}

		if metav1.HasAnnotation(pod.ObjectMeta, api.FullQueryLoggingEnabledAnnotation) == enableFQL {
			continue
		}

		features, err := rc.NodeMgmtClient.FeatureSet(pod)
		if err != nil {
			rc.ReqLogger.Error(err, "failed to verify featureset for FQL support", "pod", pod.Name)
			return result.RequeueSoon(2)
		}
		if features.Supports(httphelper.FullQuerySupport) {
			fqlEnabledForPod, err := rc.NodeMgmtClient.CallIsFullQueryLogEnabledEndpoint(pod)
			if err != nil {
				rc.ReqLogger.Error(err, "can't get whether query logging enabled for pod", "pod", pod.Name)
				return result.RequeueSoon(2)
			}
			if fqlEnabledForPod != enableFQL {
				rc.ReqLogger.Info("Modifying full query logging", "podIP", pod.Status.PodIP, "pod", pod.Name, "fqlDesiredState", enableFQL)
				if err := rc.NodeMgmtClient.CallSetFullQueryLog(pod, enableFQL); err != nil {
					rc.ReqLogger.Error(err, "couldn't modify full query logging", "podIP", pod.Status.PodIP, "pod", pod.Name)
					return result.RequeueSoon(2)
				}
			}
		} else if enableFQL {
			err := errors.New("FQL should be enabled but the Cassandra version does not support it")
			return result.Error(err)
		}

		patch := client.MergeFrom(pod.DeepCopy())
		if enableFQL {
			metav1.SetMetaDataAnnotation(&pod.ObjectMeta, api.FullQueryLoggingEnabledAnnotation, "true")
		} else {
			delete(pod.Annotations, api.FullQueryLoggingEnabledAnnotation)
		}
		if err := rc.Client.Patch(rc.Ctx, pod, patch); err != nil {
			rc.ReqLogger.Error(err, "failed to update full query logging annotation on the pod", "pod", pod.Name)
			return result.RequeueSoon(2)
		}
	}
	return result.Continue()
}
