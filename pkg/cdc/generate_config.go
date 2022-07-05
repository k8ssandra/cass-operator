package cdc

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	cassdcapi "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
)

// UpdateConfig updates the json formatted Cassandra config which incorporates the JVM options (under key additional-jvm-opts) passed into the launch scripts
// for Cassandra via the config builder.
func UpdateConfig(config json.RawMessage, cassDC cassdcapi.CassandraDatacenter) (json.RawMessage, error) {
	// Unmarshall everything into structs.
	c := configData{}
	err := json.Unmarshal(config, &c)
	if err != nil {
		return nil, err
	}
	// If CassEnvSh.AddtnlJVMOptions exists, populate a string slice with it.
	var additionalJVMOpts []string
	if c.CassEnvSh != nil && c.CassEnvSh.AddtnlJVMOptions != nil {
		additionalJVMOpts = *c.CassEnvSh.AddtnlJVMOptions
	}
	// Deal with the possibility that cassdcapi.CDCConfiguration is nil.
	CDCConfig := cassdcapi.CDCConfiguration{}
	if cassDC.Spec.CDC == nil {
		CDCConfig.Enabled = false
	} else {
		CDCConfig = *cassDC.Spec.CDC
	}
	updateCassandraYaml(&c, CDCConfig) // Add cdc_enabled: true/false to the cassandra-yaml key of the config.
	// Figure out what to do and reconcile config.CassEnvSh.AddtnlJVMOptions back to desired state per CDCConfig.
	newJVMOpts, err := updateAdditionalJVMOpts(additionalJVMOpts, CDCConfig)
	if err != nil {
		return nil, err
	}
	if c.CassEnvSh != nil {
		c.CassEnvSh.AddtnlJVMOptions = &newJVMOpts
	} else {
		c.CassEnvSh = &cassEnvSh{
			AddtnlJVMOptions: &newJVMOpts,
		}
	}
	// Marshall everything back to json and mutate the input.
	marshalled, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	return marshalled, nil
}

// updateAdditionalJVMOpts adds CDC related entries to additional-jvm-opts. Docs here https://docs.datastax.com/en/cdc-for-cassandra/cdc-apache-cassandra/$%7Bversion%7D/index.html
func updateAdditionalJVMOpts(optsSlice []string, CDCConfig cassdcapi.CDCConfiguration) ([]string, error) {
	out := removeEntryFromSlice(optsSlice, "pulsarServiceUrl")
	//Next, create an additional options entry that instantiates the settings we want.
	if CDCConfig.Enabled {
		reflectedCDCConfig := reflect.ValueOf(CDCConfig)
		t := reflectedCDCConfig.Type()
		optsSlice := []string{}
	FieldLoop:
		for i := 0; i < reflectedCDCConfig.NumField(); i++ {
			// This logic depends on the json tags from the CR mapping to the CDC agent's parameter names.
			fieldName := t.Field(i).Name
			if fieldName == "Enabled" {
				continue FieldLoop // Short circuit here as "Enabled" should not be passed on the command line.
			}
			t := reflect.TypeOf(CDCConfig)
			reflectedField, ok := t.FieldByName(fieldName)
			if !ok {
				return nil, errors.New(fmt.Sprint("could not get CDC field", fieldName))
			}
			nameTag := strings.Split(string(reflectedField.Tag.Get("json")), ",")[0]
			reflectedValue := interface{}(nil)
			// We need to get value types back from pointer types here and handle nil pointers.
			switch reflectedField.Type.Kind() {
			case reflect.Ptr:
				if !reflectedCDCConfig.Field(i).IsNil() { // We only want to append the value if it is non-nil
					reflectedValue = reflectedCDCConfig.Field(i).Elem().Interface()
					optsSlice = append(optsSlice, nameTag+"="+fmt.Sprintf("%s", reflectedValue))
				}
			case reflect.Array:
				return nil, fmt.Errorf("invalid type in CDC struct, cannot reflect field %s", fieldName)
			case reflect.Chan:
				return nil, fmt.Errorf("invalid type in CDC struct, cannot reflect %s", fieldName)
			case reflect.Func:
				return nil, fmt.Errorf("invalid type in CDC struct, cannot reflect %s", fieldName)
			case reflect.Map:
				return nil, fmt.Errorf("invalid type in CDC struct, cannot reflect %s", fieldName)
			case reflect.Interface:
				return nil, fmt.Errorf("invalid type in CDC struct, cannot reflect %s", fieldName)
			case reflect.Slice:
				return nil, fmt.Errorf("invalid type in CDC struct, cannot reflect %s", fieldName)
			default: // no need to call .Elem() when we have a value type.
				reflectedValue = reflectedCDCConfig.Field(i).Interface()
				optsSlice = append(optsSlice, nameTag+"="+fmt.Sprintf("%s", reflectedValue))
			}
		}
		CDCOpt := fmt.Sprintf("-javaagent:%s=%s", "/opt/cdc_agent/cdc-agent.jar", strings.Join(optsSlice, ","))
		out = append(out, // We need to add these two elements to the slice first, because the management agent must start before the CDC agent.
			"-javaagent:/opt/metrics-collector/lib/datastax-mcac-agent.jar",
			"-javaagent:/opt/management-api/datastax-mgmtapi-agent-0.1.0-SNAPSHOT.jar",
		)
		return append(out, CDCOpt), nil
	}
	return out, nil
}

func updateCassandraYaml(cassConfig *configData, cdcConfig cassdcapi.CDCConfiguration) {
	if cassConfig.CassandraYaml == nil {
		cassConfig.CassandraYaml = make(map[string]interface{})
	}
	switch {
	case cdcConfig.Enabled:
		cassConfig.CassandraYaml["cdc_enabled"] = true
	case !cdcConfig.Enabled:
		cassConfig.CassandraYaml["cdc_enabled"] = false
	}
}

// removeEntryFromSlice removes all an entry from a slice if it contains a substring.
func removeEntryFromSlice(optsSlice []string, substring string) []string {
	out := []string{}
	for _, optionEntry := range optsSlice {
		if !strings.Contains(optionEntry, substring) {
			out = append(out, optionEntry)
		}
	}
	return out
}
