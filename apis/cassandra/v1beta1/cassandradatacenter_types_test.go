package v1beta1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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
				ReadOnlyRootFilesystem: ptr.To[bool](true),
			},
		}

		assert.True(dc.UseClientImage())
	}
}
