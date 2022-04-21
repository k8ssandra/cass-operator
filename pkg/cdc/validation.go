package cdc

import (
	cassdcapi "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
)

// Validate validates that the cdcConfig is valid.
func Validate(cdcConfig *cassdcapi.CDCConfiguration) bool {
	if cdcConfig == nil {
		return true
	}
	switch {
	case !cdcConfig.Enabled:
		return true
	case cdcConfig.Enabled && cdcConfig.PulsarServiceUrl == nil:
		return false
	}
	return true
}
