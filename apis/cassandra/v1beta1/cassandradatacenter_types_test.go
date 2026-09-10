// Copyright DataStax, Inc.
// Please see the included license file for details.

package v1beta1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

func TestGetPortsFromCassCfg(t *testing.T) {
	tests := []struct {
		name        string
		config      []byte
		expected    CassandraConfigPorts
		expectedErr bool
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: CassandraConfigPorts{},
		},
		{
			name:     "no cassandra-yaml key",
			config:   []byte(`{"jvm-options":{}}`),
			expected: CassandraConfigPorts{},
		},
		{
			name:     "start_rpc absent",
			config:   []byte(`{"cassandra-yaml":{}}`),
			expected: CassandraConfigPorts{},
		},
		{
			name:     "start_rpc false",
			config:   []byte(`{"cassandra-yaml":{"start_rpc":false}}`),
			expected: CassandraConfigPorts{},
		},
		{
			name:     "start_rpc true uses default thrift port",
			config:   []byte(`{"cassandra-yaml":{"start_rpc":true}}`),
			expected: CassandraConfigPorts{ThriftPort: new(DefaultThriftPort)},
		},
		{
			name:     "start_rpc true with explicit rpc_port",
			config:   []byte(`{"cassandra-yaml":{"start_rpc":true,"rpc_port":9161}}`),
			expected: CassandraConfigPorts{ThriftPort: new(9161)},
		},
		{
			name:     "native_transport_port_ssl set",
			config:   []byte(`{"cassandra-yaml":{"native_transport_port_ssl":9242}}`),
			expected: CassandraConfigPorts{NativeTransportPortSSL: new(9242)},
		},
		{
			name:   "both tls-native and thrift configured",
			config: []byte(`{"cassandra-yaml":{"native_transport_port_ssl":9147,"start_rpc":true,"rpc_port":9167}}`),
			expected: CassandraConfigPorts{
				NativeTransportPortSSL: new(9147),
				ThriftPort:             new(9167),
			},
		},
		{
			name:        "native_transport_port_ssl is not a number",
			config:      []byte(`{"cassandra-yaml":{"native_transport_port_ssl":"notaport"}}`),
			expectedErr: true,
		},
		{
			name:        "start_rpc is not a bool",
			config:      []byte(`{"cassandra-yaml":{"start_rpc":"notabool"}}`),
			expectedErr: true,
		},
		{
			name:        "rpc_port is not a number",
			config:      []byte(`{"cassandra-yaml":{"start_rpc":true,"rpc_port":"notaport"}}`),
			expectedErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := GetPortsFromCassCfg(test.config)
			if test.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expected, got)
			}
		})
	}
}
