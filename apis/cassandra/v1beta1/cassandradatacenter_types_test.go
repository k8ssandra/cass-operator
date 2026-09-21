package v1beta1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsMcacEnabled(t *testing.T) {
	tests := []struct {
		name           string
		serverType     string
		version        string
		explicitEnable bool
		want           bool
	}{
		{name: "Cassandra 3.11", serverType: "cassandra", version: "3.11.17", want: true},
		{name: "Cassandra 4.0", serverType: "cassandra", version: "4.0.20", want: true},
		{name: "Cassandra 4.1", serverType: "cassandra", version: "4.1.12", want: true},
		{name: "Cassandra 5.0", serverType: "cassandra", version: "5.0.0"},
		{name: "Cassandra 5.0 with disable false", serverType: "cassandra", version: "5.0.1", explicitEnable: true},
		{name: "Cassandra 6.0", serverType: "cassandra", version: "6.0.0"},
		{name: "HCD 1.2", serverType: "hcd", version: "1.2.0", want: true},
		{name: "HCD 2.0", serverType: "hcd", version: "2.0.0"},
		{name: "HCD 2.0 patch", serverType: "hcd", version: "2.0.7"},
		{name: "DSE 6.8", serverType: "dse", version: "6.8.63", want: true},
		{name: "DSE 6.9", serverType: "dse", version: "6.9.20", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := CassandraDatacenter{Spec: CassandraDatacenterSpec{
				ServerType:             tt.serverType,
				ServerVersion:          tt.version,
				ReadOnlyRootFilesystem: new(false),
			}}
			if tt.explicitEnable {
				dc.Spec.PodTemplateSpec = &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name: "cassandra",
					Env:  []corev1.EnvVar{{Name: "MGMT_API_DISABLE_MCAC", Value: "false"}},
				}}}}
			}
			assert.Equal(t, tt.want, dc.IsMcacEnabled())

			ports, err := dc.GetContainerPorts()
			assert.NoError(t, err)
			if tt.want {
				assert.Contains(t, ports, namedPort("prometheus", 9103))
			} else {
				assert.NotContains(t, ports, namedPort("prometheus", 9103))
			}
		})
	}
}

func TestUseClientImage(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		serverType string
		version    string
		should     bool
	}{
		{
			serverType: "cassandra",
			version:    "4.1.0",
			should:     true,
		},
		{
			serverType: "cassandra",
			version:    "4.1.2",
			should:     true,
		},
		{
			serverType: "cassandra",
			version:    "5.0.0",
			should:     true,
		},
		{
			serverType: "cassandra",
			version:    "3.11.17",
			should:     false,
		},
		{
			serverType: "cassandra",
			version:    "4.0.8",
			should:     false,
		},
		{
			serverType: "dse",
			version:    "6.8.39",
			should:     false,
		},
		{
			serverType: "dse",
			version:    "6.9.0",
			should:     false,
		},
		{
			serverType: "hcd",
			version:    "1.0.0",
			should:     true,
		},
		{
			serverType: "dse",
			version:    "4.1.2",
			should:     false,
		},
	}

	for _, tt := range tests {
		dc := CassandraDatacenter{
			Spec: CassandraDatacenterSpec{
				ServerVersion: tt.version,
				ServerType:    tt.serverType,
			},
		}

		if tt.should {
			assert.True(dc.UseClientImage())
		} else {
			assert.False(dc.UseClientImage())
		}
	}
}

func TestUseClientImageEnforce(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		serverType string
		version    string
	}{
		{
			serverType: "cassandra",
			version:    "4.1.0",
		},
		{
			serverType: "cassandra",
			version:    "4.1.2",
		},
		{
			serverType: "cassandra",
			version:    "5.0.0",
		},
		{
			serverType: "cassandra",
			version:    "3.11.17",
		},
		{
			serverType: "cassandra",
			version:    "4.0.8",
		},
		{
			serverType: "dse",
			version:    "6.8.39",
		},
		{
			serverType: "dse",
			version:    "6.9.0",
		},
		{
			serverType: "hcd",
			version:    "1.0.0",
		},
		{
			serverType: "dse",
			version:    "4.1.2",
		},
	}

	for _, tt := range tests {
		dc := CassandraDatacenter{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					UseClientBuilderAnnotation: "true",
				},
			},
			Spec: CassandraDatacenterSpec{
				ServerVersion:          tt.version,
				ServerType:             tt.serverType,
				ReadOnlyRootFilesystem: new(true),
			},
		}

		assert.True(dc.UseClientImage())
	}
}
