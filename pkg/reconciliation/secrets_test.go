// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/k8ssandra/cass-operator/pkg/oplabels"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
)

func Test_buildDefaultSuperuserSecret(t *testing.T) {
	t.Run("test default superuser secret is created", func(t *testing.T) {
		dc := &api.CassandraDatacenter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "exampleDC",
				Namespace: "examplens",
			},
			Spec: api.CassandraDatacenterSpec{
				ClusterName: "exampleCluster",
				AdditionalLabels: map[string]string{
					"piclem": "add",
				},
				AdditionalAnnotations: map[string]string{"add": "annotation"},
			},
		}
		secret, err := buildDefaultSuperuserSecret(dc)
		if err != nil {
			t.Errorf("should not have returned an error %v", err)
			return
		}

		if secret.ObjectMeta.Namespace != dc.ObjectMeta.Namespace {
			t.Errorf("expected secret in namespace '%s' but was '%s", dc.ObjectMeta.Namespace, secret.ObjectMeta.Namespace)
		}

		expectedSecretName := fmt.Sprintf("%s-superuser", api.CleanupForKubernetes(dc.Spec.ClusterName))
		if secret.ObjectMeta.Name != expectedSecretName {
			t.Errorf("expected default secret name '%s' but was '%s'", expectedSecretName, secret.ObjectMeta.Name)
		}

		errors := validateCassandraUserSecretContent(dc, secret)
		if len(errors) > 0 {
			t.Errorf("expected default secret to be valid, but was not: %v", errors[0])
		}

		expectedSecretLabels := map[string]string{
			oplabels.InstanceLabel:  "cassandra-exampleCluster",
			oplabels.ManagedByLabel: oplabels.ManagedByLabelValue,
			oplabels.NameLabel:      oplabels.NameLabelValue,
			oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
			oplabels.VersionLabel:   "",
			"piclem":                "add",
		}

		if !reflect.DeepEqual(map[string]string{"add": "annotation"}, secret.Annotations) {
			t.Errorf("annotations = \n %v \n, want \n %v", secret.Annotations, map[string]string{"add": "annotation"})
		}

		if !reflect.DeepEqual(expectedSecretLabels, secret.Labels) {
			t.Errorf("labels = \n %v \n, want \n %v", secret.Labels, expectedSecretLabels)
		}
	})

	t.Run("test default superuser secret not created when explicitly defined", func(t *testing.T) {
		dc := &api.CassandraDatacenter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "exampleDC",
				Namespace: "examplens",
			},
			Spec: api.CassandraDatacenterSpec{
				ClusterName: "exampleCluster",
				// defining the following means we expect the user to provide us with a Secret
				SuperuserSecretName: "FancyPantsSecret",
			},
		}

		secret, err := buildDefaultSuperuserSecret(dc)
		if err != nil {
			t.Errorf("should not have returned an error %v", err)
			return
		}

		if secret != nil {
			t.Errorf("secret should not have been created")
		}
	})
}

func Test_validateCassandraUserSecretContent(t *testing.T) {
	var (
		name        = "datacenter-example"
		namespace   = "default"
		ClusterName = "bob"
	)

	tests := []struct {
		superuserSecret string
		secretNil       bool
		data            map[string][]byte
		valid           bool
		message         string
	}{
		{
			superuserSecret: "my-fun-secret",
			secretNil:       false,
			data: map[string][]byte{
				"username": []byte("bob-the-admin"),
				"password": []byte("12345"),
			},
			valid:   true,
			message: "validation should pass when secret contains valid username and password",
		},
		{
			superuserSecret: "my-fun-secret",
			secretNil:       false,
			data: map[string][]byte{
				"password": []byte("12345"),
			},
			valid:   false,
			message: "validation should fail when secret is missing a required key",
		},
		{
			superuserSecret: "my-fun-secret",
			secretNil:       false,
			data: map[string][]byte{
				"username": []byte("bob-the-admin"),
				"password": []byte("\xf0\x28\x8c\x28"),
			},
			valid:   false,
			message: "validation should fail when secret contains non-utf8 data",
		},
		{
			superuserSecret: "my-fun-secret",
			secretNil:       true,
			data:            nil,
			valid:           false,
			message:         "validation should fail when secret does not exists",
		},
	}

	for _, test := range tests {
		dc := &api.CassandraDatacenter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: api.CassandraDatacenterSpec{
				ClusterName:         ClusterName,
				SuperuserSecretName: test.superuserSecret,
			},
		}

		var secret *corev1.Secret = nil
		if !test.secretNil {
			secretNamespacedName := dc.GetSuperuserSecretNamespacedName()
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretNamespacedName.Name,
					Namespace: secretNamespacedName.Namespace,
				},
				Data: test.data,
			}
		}
		got := (len(validateCassandraUserSecretContent(dc, secret)) == 0)
		if got != test.valid {
			t.Error(test.message)
		}
	}
}
