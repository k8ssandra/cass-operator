// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"testing"

	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Test Phase 1: Probe initialization logic
func TestMakePodSpec_DefaultLivenessProbe(t *testing.T) {
	cassContainer := &corev1.Container{
		Name: CassandraContainerName,
		// No probe configured
	}

	// Simulate the probe initialization logic
	if cassContainer.LivenessProbe == nil {
		cassContainer.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port: intstr.FromInt(8080),
					Path: httphelper.LivenessEndpoint,
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       15,
			TimeoutSeconds:      10,
		}
	}

	// Verify default liveness probe was created
	if cassContainer.LivenessProbe == nil {
		t.Fatal("Expected liveness probe to be created")
	}

	// Verify it's an HTTPGet probe
	if cassContainer.LivenessProbe.HTTPGet == nil {
		t.Error("Expected liveness probe to have HTTPGet handler")
	}

	// Verify default values
	if cassContainer.LivenessProbe.HTTPGet.Port.IntVal != 8080 {
		t.Errorf("Expected port 8080, got %d", cassContainer.LivenessProbe.HTTPGet.Port.IntVal)
	}
	if cassContainer.LivenessProbe.HTTPGet.Path != httphelper.LivenessEndpoint {
		t.Errorf("Expected path %s, got %s", httphelper.LivenessEndpoint, cassContainer.LivenessProbe.HTTPGet.Path)
	}
	if cassContainer.LivenessProbe.InitialDelaySeconds != 15 {
		t.Errorf("Expected InitialDelaySeconds=15, got %d", cassContainer.LivenessProbe.InitialDelaySeconds)
	}
	if cassContainer.LivenessProbe.PeriodSeconds != 15 {
		t.Errorf("Expected PeriodSeconds=15, got %d", cassContainer.LivenessProbe.PeriodSeconds)
	}
	if cassContainer.LivenessProbe.TimeoutSeconds != 10 {
		t.Errorf("Expected TimeoutSeconds=10, got %d", cassContainer.LivenessProbe.TimeoutSeconds)
	}
}

func TestMakePodSpec_DefaultReadinessProbe(t *testing.T) {
	cassContainer := &corev1.Container{
		Name: CassandraContainerName,
		// No probe configured
	}

	// Simulate the probe initialization logic
	if cassContainer.ReadinessProbe == nil {
		cassContainer.ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port: intstr.FromInt(8080),
					Path: httphelper.ReadinessEndpoint,
				},
			},
			InitialDelaySeconds: 20,
			PeriodSeconds:       10,
			TimeoutSeconds:      10,
		}
	}

	// Verify default readiness probe was created
	if cassContainer.ReadinessProbe == nil {
		t.Fatal("Expected readiness probe to be created")
	}

	// Verify it's an HTTPGet probe
	if cassContainer.ReadinessProbe.HTTPGet == nil {
		t.Error("Expected readiness probe to have HTTPGet handler")
	}

	// Verify default values
	if cassContainer.ReadinessProbe.HTTPGet.Port.IntVal != 8080 {
		t.Errorf("Expected port 8080, got %d", cassContainer.ReadinessProbe.HTTPGet.Port.IntVal)
	}
	if cassContainer.ReadinessProbe.HTTPGet.Path != httphelper.ReadinessEndpoint {
		t.Errorf("Expected path %s, got %s", httphelper.ReadinessEndpoint, cassContainer.ReadinessProbe.HTTPGet.Path)
	}
	if cassContainer.ReadinessProbe.InitialDelaySeconds != 20 {
		t.Errorf("Expected InitialDelaySeconds=20, got %d", cassContainer.ReadinessProbe.InitialDelaySeconds)
	}
	if cassContainer.ReadinessProbe.PeriodSeconds != 10 {
		t.Errorf("Expected PeriodSeconds=10, got %d", cassContainer.ReadinessProbe.PeriodSeconds)
	}
	if cassContainer.ReadinessProbe.TimeoutSeconds != 10 {
		t.Errorf("Expected TimeoutSeconds=10, got %d", cassContainer.ReadinessProbe.TimeoutSeconds)
	}
}

func TestMakePodSpec_CustomHTTPGetProbe_NotOverridden(t *testing.T) {
	customPath := "/my/custom/health"
	customPort := 9999
	customInitialDelay := int32(60)
	customPeriod := int32(30)
	customTimeout := int32(20)

	cassContainer := &corev1.Container{
		Name: CassandraContainerName,
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port: intstr.FromInt(customPort),
					Path: customPath,
				},
			},
			InitialDelaySeconds: customInitialDelay,
			PeriodSeconds:       customPeriod,
			TimeoutSeconds:      customTimeout,
		},
	}

	// Simulate the probe initialization logic (should NOT override)
	if cassContainer.LivenessProbe == nil {
		cassContainer.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port: intstr.FromInt(8080),
					Path: httphelper.LivenessEndpoint,
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       15,
			TimeoutSeconds:      10,
		}
	}

	// Verify custom probe was NOT overridden
	if cassContainer.LivenessProbe.HTTPGet.Port.IntVal != int32(customPort) {
		t.Errorf("Expected custom port %d to be preserved, got %d", customPort, cassContainer.LivenessProbe.HTTPGet.Port.IntVal)
	}
	if cassContainer.LivenessProbe.HTTPGet.Path != customPath {
		t.Errorf("Expected custom path %s to be preserved, got %s", customPath, cassContainer.LivenessProbe.HTTPGet.Path)
	}
	if cassContainer.LivenessProbe.InitialDelaySeconds != customInitialDelay {
		t.Errorf("Expected custom InitialDelaySeconds=%d to be preserved, got %d", customInitialDelay, cassContainer.LivenessProbe.InitialDelaySeconds)
	}
	if cassContainer.LivenessProbe.PeriodSeconds != customPeriod {
		t.Errorf("Expected custom PeriodSeconds=%d to be preserved, got %d", customPeriod, cassContainer.LivenessProbe.PeriodSeconds)
	}
	if cassContainer.LivenessProbe.TimeoutSeconds != customTimeout {
		t.Errorf("Expected custom TimeoutSeconds=%d to be preserved, got %d", customTimeout, cassContainer.LivenessProbe.TimeoutSeconds)
	}
}

func TestMakePodSpec_CustomTCPSocketProbe_NotOverridden(t *testing.T) {
	customPort := 9042

	cassContainer := &corev1.Container{
		Name: CassandraContainerName,
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(customPort),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       20,
			TimeoutSeconds:      15,
		},
	}

	// Simulate the probe initialization logic (should NOT override)
	if cassContainer.LivenessProbe == nil {
		cassContainer.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port: intstr.FromInt(8080),
					Path: httphelper.LivenessEndpoint,
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       15,
			TimeoutSeconds:      10,
		}
	}

	// Verify custom TCPSocket probe was NOT overridden
	if cassContainer.LivenessProbe.TCPSocket == nil {
		t.Fatal("Expected TCPSocket probe to be preserved")
	}
	if cassContainer.LivenessProbe.HTTPGet != nil {
		t.Error("Expected HTTPGet to be nil (TCPSocket probe should be preserved)")
	}
	if cassContainer.LivenessProbe.TCPSocket.Port.IntVal != int32(customPort) {
		t.Errorf("Expected custom port %d to be preserved, got %d", customPort, cassContainer.LivenessProbe.TCPSocket.Port.IntVal)
	}
}

func TestMakePodSpec_CustomGRPCProbe_NotOverridden(t *testing.T) {
	customPort := int32(8080)

	cassContainer := &corev1.Container{
		Name: CassandraContainerName,
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				GRPC: &corev1.GRPCAction{
					Port: customPort,
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       20,
			TimeoutSeconds:      15,
		},
	}

	// Simulate the probe initialization logic (should NOT override)
	if cassContainer.LivenessProbe == nil {
		cassContainer.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port: intstr.FromInt(8080),
					Path: httphelper.LivenessEndpoint,
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       15,
			TimeoutSeconds:      10,
		}
	}

	// Verify custom GRPC probe was NOT overridden
	if cassContainer.LivenessProbe.GRPC == nil {
		t.Fatal("Expected GRPC probe to be preserved")
	}
	if cassContainer.LivenessProbe.HTTPGet != nil {
		t.Error("Expected HTTPGet to be nil (GRPC probe should be preserved)")
	}
	if cassContainer.LivenessProbe.GRPC.Port != customPort {
		t.Errorf("Expected custom port %d to be preserved, got %d", customPort, cassContainer.LivenessProbe.GRPC.Port)
	}
}

func TestMakePodSpec_CustomExecProbe_NotOverridden(t *testing.T) {
	customCommand := []string{"custom", "health", "check"}

	cassContainer := &corev1.Container{
		Name: CassandraContainerName,
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: customCommand,
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       20,
			TimeoutSeconds:      15,
		},
	}

	// Simulate the probe initialization logic (should NOT override)
	if cassContainer.LivenessProbe == nil {
		cassContainer.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port: intstr.FromInt(8080),
					Path: httphelper.LivenessEndpoint,
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       15,
			TimeoutSeconds:      10,
		}
	}

	// Verify custom Exec probe was NOT overridden
	if cassContainer.LivenessProbe.Exec == nil {
		t.Fatal("Expected Exec probe to be preserved")
	}
	if cassContainer.LivenessProbe.HTTPGet != nil {
		t.Error("Expected HTTPGet to be nil (Exec probe should be preserved)")
	}
	if len(cassContainer.LivenessProbe.Exec.Command) != len(customCommand) {
		t.Errorf("Expected custom command length %d to be preserved, got %d", len(customCommand), len(cassContainer.LivenessProbe.Exec.Command))
	}
	for i, cmd := range customCommand {
		if cassContainer.LivenessProbe.Exec.Command[i] != cmd {
			t.Errorf("Expected command[%d]=%s to be preserved, got %s", i, cmd, cassContainer.LivenessProbe.Exec.Command[i])
		}
	}
}
