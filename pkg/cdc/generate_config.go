package cdc

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	cassdcapi "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
)

// UpdateConfig updates the json formatted Cassandra config which incorporates the JVM options (under key additional-jvm-options) passed into the launch scripts
// for Cassandra via the config builder.
func UpdateConfig(config json.RawMessage, cassDC cassdcapi.CassandraDatacenter) (json.RawMessage, error) {
	// Unmarshall everything into structs.
	c := configData{}
	err := json.Unmarshal([]byte(config), &c)
	if err != nil {
		return nil, err
	}
	// If JvmOptions.AddtnlJVMOptions exists, populate a string slice with it.
	var additional []string
	if c.JvmOptions != nil && c.JvmOptions.AddtnlJVMOptions != nil {
		additional = *c.JvmOptions.AddtnlJVMOptions
	}
	// Deal with the possibility that cassdcapi.CDCConfiguration is nil.
	CDCConfig := cassdcapi.CDCConfiguration{}
	if cassDC.Spec.CDC == nil {
		CDCConfig.Enabled = false
	} else {
		CDCConfig = *cassDC.Spec.CDC
	}
	// Figure out what to do and reconcile config.JvmOptions.AddtnlJVMOptions back to desired state per CDCConfig.
	if CDCConfig.Enabled {
		agentPath := getAgentPath(cassDC) // get path for agent based on whether we have a DSE or Cassandra server.
		if c.JvmOptions != nil {
			newValue, err := updateCDC(additional, CDCConfig, agentPath)
			if err != nil {
				return nil, err
			}
			c.JvmOptions.AddtnlJVMOptions = &newValue
		} else {
			newValue, err := updateCDC(additional, CDCConfig, agentPath)
			if err != nil {
				return nil, err
			}
			c.JvmOptions = &jvmOptions{
				AddtnlJVMOptions: &newValue,
			}
		}
	} else {
		newValue := disableCDC(additional)
		if c.JvmOptions != nil {
			c.JvmOptions.AddtnlJVMOptions = &newValue
		} else if len(newValue) > 0 {
			c.JvmOptions = &jvmOptions{
				AddtnlJVMOptions: &newValue,
			}
		}
	}
	// Marshall everything back to json and mutate the input.
	marshalled, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(marshalled), nil
}

// disableCDC adds CDC related entries to additional-jvm-opts. Docs here https://docs.datastax.com/en/cdc-for-cassandra/cdc-apache-cassandra/$%7Bversion%7D/index.html
func updateCDC(optsSlice []string, CDCConfig cassdcapi.CDCConfiguration, agentPath string) ([]string, error) {
	out := disableCDC(optsSlice)
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
		CDCOpt := fmt.Sprintf("-javaagent:%s=%s", agentPath, strings.Join(optsSlice, ","))
		return append(out, CDCOpt), nil
	}
	return out, nil
}

// disableCDC removes all CDC related entries from additional-jvm-opts. Docs here https://docs.datastax.com/en/cdc-for-cassandra/cdc-apache-cassandra/$%7Bversion%7D/index.html
func disableCDC(optsSlice []string) []string {
	found := false
	out := []string{}
	for i, optionEntry := range optsSlice {
		if strings.Contains(optionEntry, "pulsarServiceUrl") {
			out = append(optsSlice[:i], optsSlice[i+1:]...)
			found = true
		}
	}
	if !found {
		return optsSlice
	}
	return out
}

func getAgentPath(dc cassdcapi.CassandraDatacenter) string {
	if dc.Spec.ServerType == "cassandra" && strings.HasPrefix(dc.Spec.ServerVersion, "3") {
		return fmt.Sprintf("/opt/cdc_agent/agent-c3-%s-all", CDCAgentVer)
	} else if dc.Spec.ServerType == "cassandra" && strings.HasPrefix(dc.Spec.ServerVersion, "4") {
		return fmt.Sprintf("/opt/cdc_agent/agent-c4-%s-all", CDCAgentVer)
	} else {
		return fmt.Sprintf("/opt/cdc_agent/agent-dse4-%s-all", CDCAgentVer)
	}
}
