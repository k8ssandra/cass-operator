// Copyright DataStax, Inc.
// Please see the included license file for details.

package httphelper

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// Phase 4: validateProbe validates that a probe complies with Kubernetes specifications
// Returns a list of validation errors, or empty slice if probe is valid
func validateProbe(probe *corev1.Probe, probeType string) []error {
	var errors []error

	if probe == nil {
		return errors // nil probe is valid (means no probe)
	}

	// Count how many mechanisms are set
	mechanismCount := 0
	if probe.Exec != nil {
		mechanismCount++
	}
	if probe.HTTPGet != nil {
		mechanismCount++
	}
	if probe.TCPSocket != nil {
		mechanismCount++
	}
	if probe.GRPC != nil {
		mechanismCount++
	}

	// Validate exactly one mechanism is defined
	if mechanismCount == 0 {
		errors = append(errors, fmt.Errorf("%s probe has no mechanism defined (must define exactly one of: exec, httpGet, tcpSocket, or grpc)", probeType))
	} else if mechanismCount > 1 {
		errors = append(errors, fmt.Errorf("%s probe has multiple mechanisms defined (must define exactly one of: exec, httpGet, tcpSocket, or grpc)", probeType))
	}

	// Validate successThreshold for liveness/startup probes
	// Per Kubernetes spec: "Must be 1 for liveness and startup Probes"
	if (probeType == "liveness" || probeType == "startup") && probe.SuccessThreshold != 0 && probe.SuccessThreshold != 1 {
		errors = append(errors, fmt.Errorf("%s probe successThreshold must be 1 (got %d)", probeType, probe.SuccessThreshold))
	}

	// Validate timing values are positive
	if probe.TimeoutSeconds < 0 {
		errors = append(errors, fmt.Errorf("%s probe timeoutSeconds must be >= 0 (got %d)", probeType, probe.TimeoutSeconds))
	}
	if probe.PeriodSeconds < 0 {
		errors = append(errors, fmt.Errorf("%s probe periodSeconds must be >= 0 (got %d)", probeType, probe.PeriodSeconds))
	}
	if probe.InitialDelaySeconds < 0 {
		errors = append(errors, fmt.Errorf("%s probe initialDelaySeconds must be >= 0 (got %d)", probeType, probe.InitialDelaySeconds))
	}
	if probe.FailureThreshold < 0 {
		errors = append(errors, fmt.Errorf("%s probe failureThreshold must be >= 0 (got %d)", probeType, probe.FailureThreshold))
	}
	if probe.SuccessThreshold < 0 {
		errors = append(errors, fmt.Errorf("%s probe successThreshold must be >= 0 (got %d)", probeType, probe.SuccessThreshold))
	}

	// Validate port numbers if applicable
	if probe.HTTPGet != nil {
		if probe.HTTPGet.Port.IntVal < 1 || probe.HTTPGet.Port.IntVal > 65535 {
			errors = append(errors, fmt.Errorf("%s probe httpGet port must be in range 1-65535 (got %d)", probeType, probe.HTTPGet.Port.IntVal))
		}
	}
	if probe.TCPSocket != nil {
		if probe.TCPSocket.Port.IntVal < 1 || probe.TCPSocket.Port.IntVal > 65535 {
			errors = append(errors, fmt.Errorf("%s probe tcpSocket port must be in range 1-65535 (got %d)", probeType, probe.TCPSocket.Port.IntVal))
		}
	}
	if probe.GRPC != nil {
		if probe.GRPC.Port < 1 || probe.GRPC.Port > 65535 {
			errors = append(errors, fmt.Errorf("%s probe grpc port must be in range 1-65535 (got %d)", probeType, probe.GRPC.Port))
		}
	}

	return errors
}

// ValidateLivenessProbe validates a liveness probe
func ValidateLivenessProbe(probe *corev1.Probe) []error {
	return validateProbe(probe, "liveness")
}

// ValidateReadinessProbe validates a readiness probe
func ValidateReadinessProbe(probe *corev1.Probe) []error {
	return validateProbe(probe, "readiness")
}

// ValidateStartupProbe validates a startup probe
func ValidateStartupProbe(probe *corev1.Probe) []error {
	return validateProbe(probe, "startup")
}
