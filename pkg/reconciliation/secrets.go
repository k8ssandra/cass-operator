// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"unicode/utf8"

	"github.com/k8ssandra/cass-operator/pkg/oplabels"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/utils"
)

func generateUtf8Password() (string, error) {
	// Note that bcrypt has a maximum password length of 55 characters:
	//
	// https://security.stackexchange.com/questions/39849/does-bcrypt-have-a-maximum-password-length
	//
	// Since 1 ASCII character equals one byte in UTF-8, and base64
	// encoding generates 4 bytes (4 ASCII characters) for every 3
	// bytes encoded, we have:
	//
	//   55 encoded bytes * (3 unencoded bytes / 4 encoded bytes) = 41.25 unencoded bytes
	//
	// So we must generate 41 bytes or less below to ensure we end up
	// with a password no greater than 55 characters.
	buf := make([]byte, 40)
	_, err := rand.Read(buf)
	if err != nil {
		return "", fmt.Errorf("failed to generate password: %w", err)
	}

	// Now that we have some random bytes, we need to turn it into valid
	// utf8 characters
	//
	// Example output:
	//
	//   7GOZOdMuQdjzJceJyla/72FkX0ymJDNNEyKKWVUxTP4IXtAUzYp8U0z0d8Wqh+p7J+K+D0NepgoEjqA79bBC6UkVtcorTFH+BBYaAetd3FsZdZ6V5Nn+UQ/VhpGNxU0fb7FOVg
	//
	password := base64.RawURLEncoding.EncodeToString(buf)

	return password, nil
}

func buildDefaultSuperuserSecret(dc *api.CassandraDatacenter) (*corev1.Secret, error) {
	var secret *corev1.Secret = nil

	if dc.ShouldGenerateSuperuserSecret() {
		secretNamespacedName := dc.GetSuperuserSecretNamespacedName()
		secret = &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        secretNamespacedName.Name,
				Namespace:   secretNamespacedName.Namespace,
				Labels:      dc.GetDatacenterLabels(),
				Annotations: make(map[string]string),
			},
		}
		oplabels.AddOperatorMetadata(&secret.ObjectMeta, dc)
		username := api.CleanupForKubernetes(dc.Spec.ClusterName) + "-superuser"
		password, err := generateUtf8Password()
		if err != nil {
			return nil, fmt.Errorf("failed to generate superuser password: %w", err)
		}

		secret.Data = map[string][]byte{
			"username": []byte(username),
			"password": []byte(password),
		}
	}

	return secret, nil
}

func (rc *ReconciliationContext) retrieveSecret(secretNamespacedName types.NamespacedName) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretNamespacedName.Name,
			Namespace: secretNamespacedName.Namespace,
		},
	}

	err := rc.Client.Get(
		rc.Ctx,
		secretNamespacedName,
		secret)
	if err != nil {
		return nil, err
	}

	return secret, nil
}

func (rc *ReconciliationContext) retrieveSuperuserSecret() (*corev1.Secret, error) {
	dc := rc.Datacenter
	secretNamespacedName := dc.GetSuperuserSecretNamespacedName()
	return rc.retrieveSecret(secretNamespacedName)
}

func (rc *ReconciliationContext) retrieveSuperuserSecretOrCreateDefault() error {
	dc := rc.Datacenter

	_, retrieveErr := rc.retrieveSuperuserSecret()
	if retrieveErr != nil {
		if errors.IsNotFound(retrieveErr) {
			secret, err := buildDefaultSuperuserSecret(dc)

			if err == nil && secret == nil {
				return retrieveErr
			}

			if err == nil {
				err = rc.Client.Create(rc.Ctx, secret)
			}

			if err != nil {
				return fmt.Errorf("failed to create default superuser secret: %w", err)
			}
		} else {
			return retrieveErr
		}
	}

	return nil
}

func (rc *ReconciliationContext) createInternodeCACredential() (*corev1.Secret, error) {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        rc.keystoreCASecret().Name,
			Namespace:   rc.keystoreCASecret().Namespace,
			Labels:      rc.Datacenter.GetDatacenterLabels(),
			Annotations: make(map[string]string),
		},
	}

	oplabels.AddOperatorMetadata(&secret.ObjectMeta, rc.Datacenter)

	if keypem, certpem, err := utils.GetNewCAandKey(fmt.Sprintf("%s-ca-keystore", rc.Datacenter.Name), rc.Datacenter.Namespace); err == nil {
		secret.Data = map[string][]byte{
			"key":  []byte(keypem),
			"cert": []byte(certpem),
		}
		return secret, nil
	} else {
		return nil, err
	}
}

func (rc *ReconciliationContext) createCABootstrappingSecret(jksBlob []byte) error {
	if _, err := rc.retrieveSecret(types.NamespacedName{
		Name:      fmt.Sprintf("%s-keystore", rc.Datacenter.Name),
		Namespace: rc.Datacenter.Namespace,
	}); err == nil {
		return nil
	}

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("%s-keystore", rc.Datacenter.Name),
			Namespace:   rc.Datacenter.Namespace,
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
	}
	secret.Data = map[string][]byte{
		"node-keystore.jks": jksBlob,
	}

	oplabels.AddOperatorMetadata(&secret.ObjectMeta, rc.Datacenter)

	return rc.Client.Create(rc.Ctx, secret)
}

func (rc *ReconciliationContext) keystoreCASecret() types.NamespacedName {
	return types.NamespacedName{Name: fmt.Sprintf("%s-ca-keystore", rc.Datacenter.Name), Namespace: rc.Datacenter.Namespace}
}

func (rc *ReconciliationContext) retrieveInternodeCredentialSecretOrCreateDefault() (*corev1.Secret, error) {
	secret, retrieveErr := rc.retrieveSecret(rc.keystoreCASecret())
	if retrieveErr != nil {
		if errors.IsNotFound(retrieveErr) {
			var err error
			secret, err = rc.createInternodeCACredential()

			if err == nil && secret == nil {
				return nil, retrieveErr
			}

			if err == nil {
				err = rc.Client.Create(rc.Ctx, secret)
			}

			if err != nil {
				return nil, fmt.Errorf("failed to create internode CA credential: %w", err)
			}
		} else {
			return nil, retrieveErr
		}
	}

	_, retrieveBootStrappingSecretErr := rc.retrieveSecret(types.NamespacedName{
		Name:      fmt.Sprintf("%s-keystore", rc.Datacenter.Name),
		Namespace: rc.Datacenter.Namespace,
	})
	if retrieveBootStrappingSecretErr != nil {
		if errors.IsNotFound(retrieveBootStrappingSecretErr) {
			var jksBlob []byte
			jksBlob, err := utils.GenerateJKS(secret, rc.Datacenter.Name, rc.Datacenter.Name)
			if err == nil {
				err = rc.createCABootstrappingSecret(jksBlob)
			}

			if err != nil {
				return nil, fmt.Errorf("failed to create default superuser secret: %w", err)
			}
		} else {
			return nil, retrieveBootStrappingSecretErr
		}
	}

	return secret, nil
}

// Helper function that is easier to test
func validateCassandraUserSecretContent(secret *corev1.Secret) []error {
	var errs []error

	if secret != nil {
		namespacedName := types.NamespacedName{
			Name:      secret.Name,
			Namespace: secret.Namespace,
		}
		errorPrefix := fmt.Sprintf("Validation failed for user secret: %s", namespacedName.String())

		for _, key := range []string{"username", "password"} {
			value, ok := secret.Data[key]
			if !ok {
				errs = append(errs, fmt.Errorf("%s Missing key: %s", errorPrefix, key))
			} else if !utf8.Valid(value) {
				errs = append(errs, fmt.Errorf("%s Key did not have valid utf8 value: %s", errorPrefix, key))
			}
		}
	} else {
		errs = append(errs, fmt.Errorf("validation failed due to a missing secret"))
	}

	return errs
}

func (rc *ReconciliationContext) validateSuperuserSecret() []error {
	dc := rc.Datacenter
	secret, err := rc.retrieveSuperuserSecret()
	if err != nil {
		if errors.IsNotFound(err) {
			if dc.ShouldGenerateSuperuserSecret() {
				return []error{}
			} else {
				return []error{
					fmt.Errorf("could not load superuser secret for CassandraCluster: %s",
						dc.GetSuperuserSecretNamespacedName().String()),
				}
			}
		} else {
			return []error{fmt.Errorf("validation of superuser secret failed due to an error: %w", err)}
		}
	}
	return validateCassandraUserSecretContent(secret)
}

func (rc *ReconciliationContext) validateCassandraUserSecrets() []error {
	users := rc.Datacenter.Spec.Users
	dc := rc.Datacenter
	errs := []error{}

	for _, user := range users {
		secretName := user.SecretName
		namespace := dc.Namespace
		secretKey := types.NamespacedName{
			Name:      secretName,
			Namespace: namespace,
		}

		secret, err := rc.retrieveSecret(secretKey)
		if err != nil {
			errs = append(errs, fmt.Errorf("validation of user secret failed due to an error: %w", err))
			continue
		}

		errs = append(errs, validateCassandraUserSecretContent(secret)...)
	}

	return errs
}
