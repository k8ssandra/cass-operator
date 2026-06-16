// Copyright DataStax, Inc.
// Please see the included license file for details.

package httphelper

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestValidateProbe_NilProbe(t *testing.T) {
	errors := validateProbe(nil, "liveness")
	if len(errors) != 0 {
		t.Errorf("Expected no errors for nil probe, got %d errors", len(errors))
	}
}

func TestValidateProbe_NoMechanism(t *testing.T) {
	probe := &corev1.Probe{}
	errors := validateProbe(probe, "liveness")
	if len(errors) != 1 {
		t.Errorf("Expected 1 error for probe with no mechanism, got %d", len(errors))
	}
	if len(errors) > 0 && errors[0].Error() != "liveness probe has no mechanism defined (must define exactly one of: exec, httpGet, tcpSocket, or grpc)" {
		t.Errorf("Unexpected error message: %v", errors[0])
	}
}

func TestValidateProbe_MultipleMechanisms(t *testing.T) {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port: intstr.FromInt(8080),
			},
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt(8080),
			},
		},
	}
	errors := validateProbe(probe, "liveness")
	if len(errors) != 1 {
		t.Errorf("Expected 1 error for probe with multiple mechanisms, got %d", len(errors))
	}
	if len(errors) > 0 && errors[0].Error() != "liveness probe has multiple mechanisms defined (must define exactly one of: exec, httpGet, tcpSocket, or grpc)" {
		t.Errorf("Unexpected error message: %v", errors[0])
	}
}

func TestValidateProbe_ValidHTTPGet(t *testing.T) {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port: intstr.FromInt(8080),
				Path: "/health",
			},
		},
		InitialDelaySeconds: 10,
		PeriodSeconds:       5,
		TimeoutSeconds:      3,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}
	errors := validateProbe(probe, "liveness")
	if len(errors) != 0 {
		t.Errorf("Expected no errors for valid HTTPGet probe, got %d errors: %v", len(errors), errors)
	}
}

func TestValidateProbe_ValidTCPSocket(t *testing.T) {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt(9042),
			},
		},
		InitialDelaySeconds: 10,
		PeriodSeconds:       5,
		TimeoutSeconds:      3,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}
	errors := validateProbe(probe, "readiness")
	if len(errors) != 0 {
		t.Errorf("Expected no errors for valid TCPSocket probe, got %d errors: %v", len(errors), errors)
	}
}

func TestValidateProbe_ValidGRPC(t *testing.T) {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			GRPC: &corev1.GRPCAction{
				Port: 8080,
			},
		},
		InitialDelaySeconds: 10,
		PeriodSeconds:       5,
		TimeoutSeconds:      3,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}
	errors := validateProbe(probe, "liveness")
	if len(errors) != 0 {
		t.Errorf("Expected no errors for valid GRPC probe, got %d errors: %v", len(errors), errors)
	}
}

func TestValidateProbe_ValidExec(t *testing.T) {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"test", "-f", "/tmp/healthy"},
			},
		},
		InitialDelaySeconds: 10,
		PeriodSeconds:       5,
		TimeoutSeconds:      3,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}
	errors := validateProbe(probe, "liveness")
	if len(errors) != 0 {
		t.Errorf("Expected no errors for valid Exec probe, got %d errors: %v", len(errors), errors)
	}
}

func TestValidateProbe_InvalidSuccessThresholdForLiveness(t *testing.T) {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port: intstr.FromInt(8080),
			},
		},
		SuccessThreshold: 3, // Invalid for liveness
	}
	errors := validateProbe(probe, "liveness")
	if len(errors) != 1 {
		t.Errorf("Expected 1 error for invalid successThreshold, got %d", len(errors))
	}
}

func TestValidateProbe_ValidSuccessThresholdForReadiness(t *testing.T) {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port: intstr.FromInt(8080),
			},
		},
		SuccessThreshold: 3, // Valid for readiness
	}
	errors := validateProbe(probe, "readiness")
	if len(errors) != 0 {
		t.Errorf("Expected no errors for valid readiness probe with successThreshold=3, got %d errors: %v", len(errors), errors)
	}
}

func TestValidateProbe_InvalidPortNumber(t *testing.T) {
	tests := []struct {
		name      string
		probe     *corev1.Probe
		probeType string
	}{
		{
			name: "HTTPGet port too low",
			probe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Port: intstr.FromInt(0),
					},
				},
			},
			probeType: "liveness",
		},
		{
			name: "HTTPGet port too high",
			probe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Port: intstr.FromInt(65536),
					},
				},
			},
			probeType: "liveness",
		},
		{
			name: "TCPSocket port too low",
			probe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					TCPSocket: &corev1.TCPSocketAction{
						Port: intstr.FromInt(0),
					},
				},
			},
			probeType: "readiness",
		},
		{
			name: "GRPC port too high",
			probe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					GRPC: &corev1.GRPCAction{
						Port: 65536,
					},
				},
			},
			probeType: "liveness",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validateProbe(tt.probe, tt.probeType)
			if len(errors) == 0 {
				t.Errorf("Expected error for invalid port number, got none")
			}
		})
	}
}

func TestValidateProbe_NegativeTimingValues(t *testing.T) {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port: intstr.FromInt(8080),
			},
		},
		InitialDelaySeconds: -1,
		PeriodSeconds:       -1,
		TimeoutSeconds:      -1,
		SuccessThreshold:    -1,
		FailureThreshold:    -1,
	}
	errors := validateProbe(probe, "liveness")
	// Should have 5 errors (one for each negative value)
	if len(errors) < 5 {
		t.Errorf("Expected at least 5 errors for negative timing values, got %d", len(errors))
	}
}

func TestValidateLivenessProbe(t *testing.T) {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port: intstr.FromInt(8080),
			},
		},
	}
	errors := ValidateLivenessProbe(probe)
	if len(errors) != 0 {
		t.Errorf("Expected no errors for valid liveness probe, got %d errors: %v", len(errors), errors)
	}
}

func TestValidateReadinessProbe(t *testing.T) {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt(9042),
			},
		},
	}
	errors := ValidateReadinessProbe(probe)
	if len(errors) != 0 {
		t.Errorf("Expected no errors for valid readiness probe, got %d errors: %v", len(errors), errors)
	}
}

func TestValidateStartupProbe(t *testing.T) {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"test"},
			},
		},
	}
	errors := ValidateStartupProbe(probe)
	if len(errors) != 0 {
		t.Errorf("Expected no errors for valid startup probe, got %d errors: %v", len(errors), errors)
	}
}
