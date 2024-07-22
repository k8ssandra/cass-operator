package utils

import (
	"fmt"
	"os"
	"strings"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// StringSet helper functions
type StringSet map[string]bool

func UnionStringSet(a, b StringSet) StringSet {
	result := StringSet{}
	for _, m := range []StringSet{a, b} {
		for k := range m {
			result[k] = true
		}
	}
	return result
}

func SubtractStringSet(a, b StringSet) StringSet {
	result := StringSet{}
	for k := range a {
		if !b[k] {
			result[k] = true
		}
	}
	return result
}

func IntersectionStringSet(a, b StringSet) StringSet {
	result := StringSet{}
	for k, v := range a {
		if v && b[k] {
			result[k] = true
		}
	}
	return result
}

//
// Migrated from operator-sdk, these are internal in newer versions
//

// ForceRunModeEnv indicates if the operator should be forced to run in either local
// or cluster mode (currently only used for local mode)
var ForceRunModeEnv = "OSDK_FORCE_RUN_MODE"

type RunModeType string

const (
	LocalRunMode   RunModeType = "local"
	ClusterRunMode RunModeType = "cluster"
)

var log = logf.Log.WithName("k8sutil")

const (
	// WatchNamespaceEnvVar is the constant for env variable WATCH_NAMESPACE
	// which is the namespace where the watch activity happens.
	// this value is empty if the operator is running with clusterScope.
	WatchNamespaceEnvVar = "WATCH_NAMESPACE"
)

// GetWatchNamespace returns the namespace the operator should be watching for changes
func GetWatchNamespace() (string, error) {
	ns, found := os.LookupEnv(WatchNamespaceEnvVar)
	if !found {
		return "", fmt.Errorf("%s must be set", WatchNamespaceEnvVar)
	}
	return ns, nil
}

// ErrNoNamespace indicates that a namespace could not be found for the current
// environment
var ErrNoNamespace = fmt.Errorf("namespace not found for current environment")

// ErrRunLocal indicates that the operator is set to run in local mode (this error
// is returned by functions that only work on operators running in cluster mode)
var ErrRunLocal = fmt.Errorf("operator run mode forced to local")

// GetOperatorNamespace returns the namespace the operator should be running in.
func GetOperatorNamespace() (string, error) {
	if isRunModeLocal() {
		return "", ErrRunLocal
	}
	nsBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNoNamespace
		}
		return "", err
	}
	ns := strings.TrimSpace(string(nsBytes))
	log.V(1).Info("Found namespace", "Namespace", ns)
	return ns, nil
}

func isRunModeLocal() bool {
	return os.Getenv(ForceRunModeEnv) == string(LocalRunMode)
}
