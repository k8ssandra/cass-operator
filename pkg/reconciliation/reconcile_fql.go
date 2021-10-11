package reconciliation

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/k8ssandra/cass-operator/pkg/internal/result"
)

func parseFQLFromConfig(rc *ReconciliationContext) (bool, result.ReconcileResult) {
	// Should FQL be enabled according to the CassDC config?
	shouldFQLBeEnabled := false
	dc := rc.GetDatacenter()
	if dc.Spec.Config != nil {
		var dcConfig map[string]interface{}
		if err := json.Unmarshal(dc.Spec.Config, &dcConfig); err != nil {
			rc.ReqLogger.Error(err, "error unmarshalling DC config JSON")
			return false, result.Error(err)
		}
		casYaml, found := dcConfig["cassandra-yaml"]
		if !found {
			return false, result.Continue()
		}
		casYamlMap, ok := casYaml.(map[string]interface{})
		if !ok {
			err := fmt.Errorf("type casting error")
			rc.ReqLogger.Error(err, "couldn't cast cassandra-yaml value from config to map[string]interface{}")
			return false, result.Error(err)
		}
		if _, found := casYamlMap["full_query_logging_options"]; found {
			serverMajorVersion, err := strconv.ParseInt(strings.Split(dc.Spec.ServerVersion, ".")[0], 10, 8)
			if err != nil {
				rc.ReqLogger.Error(err, "error parsing server major version. Can't enable full query logging without knowing this")
				return false, result.Error(err)
			}
			if serverMajorVersion < 4 {
				err := fmt.Errorf("full query logging only supported on OSS Cassandra 4x+")
				rc.ReqLogger.Error(err, "full_query_logging_options is defined in Cassandra config, it is not supported on the version of Cassandra you are running")
				return false, result.Error(err)
			}
			rc.ReqLogger.Info("full_query_logging_options is defined in Cassandra config, we will try to enable it via the management API")
			shouldFQLBeEnabled = true
		}
	}
	return shouldFQLBeEnabled, result.Continue()
}

func SetFullQueryLogging(rc *ReconciliationContext, enableFQL bool) result.ReconcileResult {
	podList, err := rc.listPods(rc.Datacenter.GetClusterLabels())
	if err != nil {
		rc.ReqLogger.Error(err, "error listing all pods in the cluster to progress full query logging reconciliation")
		return result.RequeueSoon(2)
	}
	for _, podPtr := range PodPtrsFromPodList(podList) {
		fqlEnabledForPod, err := rc.NodeMgmtClient.CallIsFullQueryLogEnabledEndpoint(podPtr)
		if err != nil {
			rc.ReqLogger.Error(err, "can't get whether query logging enabled for pod ", "podName", podPtr.Name)
			return result.RequeueSoon(2)
		}
		rc.ReqLogger.Info("full query logging status:", "isEnabled", fqlEnabledForPod, "shouldBeEnabled", enableFQL)
		if fqlEnabledForPod != enableFQL {
			rc.ReqLogger.Info("Setting full query logging on ", "podIP", podPtr.Status.PodIP, "podName", podPtr.Name, "fqlDesiredState", enableFQL)
			err := rc.NodeMgmtClient.CallSetFullQueryLog(podPtr, enableFQL)
			if err != nil {
				rc.ReqLogger.Error(err, "couldn't enable full query logging on ", "podIP", podPtr.Status.PodIP, "podName", podPtr.Name)
				return result.RequeueSoon(2)
			}
		}
	}
	return result.Continue()
}
