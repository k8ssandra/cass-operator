package reconciliation

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	api "github.com/k8ssandra/cass-operator/api/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/internal/result"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/pkg/util/hash"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CheckConfigSecret When the ConfigSecret property is set, take the configuration from the
// specified secret and add to the datacenter configuration secret. The datacenter
// configuration is created by cass-operator. A second secret is used because cass-operator
// adds additional properties to the configuration, and we do not want to write that
// updated configuration back to the user's secret since we do not own it.
func (rc *ReconciliationContext) CheckConfigSecret() result.ReconcileResult {
	rc.ReqLogger.Info("reconcile_racks::CheckConfigSecret")

	if len(rc.Datacenter.Spec.ConfigSecret) == 0 {
		return result.Continue()
	}

	key := types.NamespacedName{Namespace: rc.Datacenter.Namespace, Name: rc.Datacenter.Spec.ConfigSecret}
	secret, err := rc.retrieveSecret(key)

	if err != nil {
		rc.ReqLogger.Error(err, "failed to get config secret", "ConfigSecret", key.Name)
		return result.Error(err)
	}

	if err := rc.checkDatacenterNameAnnotation(secret); err != nil {
		rc.ReqLogger.Error(err, "annotation check for config secret failed", "ConfigSecret", secret.Name)
	}

	config, err := getConfigFromConfigSecret(rc.Datacenter, secret)
	if err != nil {
		rc.ReqLogger.Error(err, "failed to get json config from secret", "ConfigSecret", rc.Datacenter.Spec.ConfigSecret)
		return result.Error(err)
	}

	secretName := getDatacenterConfigSecretName(rc.Datacenter)
	dcConfigSecret, exists, err := rc.getDatacenterConfigSecret(secretName)
	if err != nil {
		rc.ReqLogger.Error(err, "failed to get datacenter config secret")
		return result.Error(err)
	}

	storedConfig, found := dcConfigSecret.Data["config"]
	if !(found && bytes.Equal(storedConfig, config)) {
		if err := rc.updateConfigHashAnnotation(dcConfigSecret); err != nil {
			rc.ReqLogger.Error(err, "failed to update config hash annotation")
			return result.Error(err)
		}

		rc.ReqLogger.Info("updating datacenter config secret", "ConfigSecret", dcConfigSecret.Name)
		dcConfigSecret.Data["config"] = config

		if exists {
			if err := rc.Client.Update(rc.Ctx, dcConfigSecret); err != nil {
				rc.ReqLogger.Error(err, "failed to update datacenter config secret", "ConfigSecret", dcConfigSecret.Name)
				return result.Error(err)
			}
		}
		if err := rc.Client.Create(rc.Ctx, dcConfigSecret); err != nil {
			rc.ReqLogger.Error(err, "failed to create datacenter config secret", "ConfigSecret", dcConfigSecret.Name)
			return result.Error(err)
		}
	}

	return result.Continue()
}

// checkDatacenterNameAnnotation Checks to see if the secret has the datacenter annotation.
// If the secret does not have the annotation, it is added, and the secret is patched. The
// secret should be the one specifiied by ConfigSecret.
func (rc *ReconciliationContext) checkDatacenterNameAnnotation(secret *corev1.Secret) error {
	if v, ok := secret.Annotations[api.DatacenterAnnotation]; ok && v == rc.Datacenter.Name {
		return nil
	}

	patch := client.MergeFrom(secret.DeepCopy())
	secret.Annotations[api.DatacenterAnnotation] = rc.Datacenter.Name
	return rc.Client.Patch(rc.Ctx, secret, patch)
}

// updateConfigHashAnnotation Adds the config hash annotation to the datacenter. The value
// of the annotation is a hash of the secret. The datacenter is then patched. Note that the
// secret should be the datacenter secret created by the operator, NOT the secret specified
// by ConfigSecret.
func (rc *ReconciliationContext) updateConfigHashAnnotation(secret *corev1.Secret) error {
	rc.ReqLogger.Info("updating config hash annotation")

	hasher := sha256.New()
	hash.DeepHashObject(hasher, secret)
	hashBytes := hasher.Sum([]byte{})
	b64Hash := base64.StdEncoding.EncodeToString(hashBytes)

	patch := client.MergeFrom(rc.Datacenter.DeepCopy())
	rc.Datacenter.Annotations[api.ConfigHashAnnotation] = b64Hash

	return rc.Client.Patch(rc.Ctx, rc.Datacenter, patch)
}

// getConfigFromConfigSecret Generates the JSON with properties added by cass-operator.
func getConfigFromConfigSecret(dc *api.CassandraDatacenter, secret *corev1.Secret) ([]byte, error) {
	if b, found := secret.Data["config"]; found {
		jsonConfig, err := dc.GetConfigAsJSON(b)
		if err == nil {
			return []byte(jsonConfig), nil
		} else {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("invalid config secret %s: config property is required", dc.Spec.ConfigSecret)
	}
}

// getDatacenterConfigSecretName The format is clusterName-dcName-config
func getDatacenterConfigSecretName(dc *api.CassandraDatacenter) string {
	return dc.Spec.ClusterName + "-" + dc.Name + "-config"
}

// getDatacenterConfigSecret Fetches the secret from the api server or creates a new secret
// if one is not already stored. The bool return parameter is true if the api server has
// the secret, false if a secret has to be created. This function does not persist the new
// secret. That is the caller's responsibility. Lastly, note that this NOT the secret
// specified by the ConfigSecret property. This is the secret created based on the
// contents of ConfigSecret.
func (rc *ReconciliationContext) getDatacenterConfigSecret(name string) (*corev1.Secret, bool, error) {
	key := types.NamespacedName{Namespace: rc.Datacenter.Namespace, Name: name}
	secret, err := rc.retrieveSecret(key)

	if err == nil {
		return secret, true, nil
	} else if errors.IsNotFound(err) {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: key.Namespace,
				Name:      name,
			},
			Data: map[string][]byte{},
		}

		if err = rc.SetDatacenterAsOwner(secret); err != nil {
			return nil, false, err
		}

		return secret, false, nil
	} else {
		return nil, false, err
	}
}
