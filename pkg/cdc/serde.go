// This file contains horrible things that allow us to marshal/unmarshal the parts of the config we care about to/from json, without losing the fields we don't
// care about.
package cdc

import "encoding/json"

type configData struct {
	CassEnvSh     *cassEnvSh `json:"cassandra-env-sh,omitempty"`
	CassandraYaml map[string]interface{}
	UnknownFields map[string]interface{}
}

func (c *configData) UnmarshalJSON(data []byte) error {
	intermediate := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(data), &intermediate); err != nil {
		return err
	}
	// If jvm-options key exists, parse, add to c.CassEnvSh field, delete from intermediate map.
	if jvmOptsUnparsed, exists := intermediate["cassandra-env-sh"]; exists {
		parsedjvmOpts := cassEnvSh{} // First parse the known field "jvm-options" into the known struct cassEnvSh{}
		if err := json.Unmarshal(jvmOptsUnparsed, &parsedjvmOpts); err != nil {
			return err
		}
		c.CassEnvSh = &parsedjvmOpts
		delete(intermediate, "cassandra-env-sh")
	}
	// If cassandra-yaml key exists, parse, add to c.CassEnvSh field, delete from intermediate map.
	if cassYamlUnparsed, exists := intermediate["cassandra-yaml"]; exists {
		parsedCassYaml := make(map[string]interface{}) // First parse the known field "jvm-options" into the known struct cassEnvSh{}
		if err := json.Unmarshal(cassYamlUnparsed, &parsedCassYaml); err != nil {
			return err
		}
		c.CassandraYaml = parsedCassYaml
		delete(intermediate, "cassandra-yaml")
	}
	// Now parse the remaining fields as a map[string]interface{}.
	unknownFields := make(map[string]interface{})
	for k, v := range intermediate {
		var tmp interface{}
		if err := json.Unmarshal(v, &tmp); err != nil {
			return err
		}
		unknownFields[k] = tmp
	}
	c.UnknownFields = unknownFields
	return nil
}

func (c configData) MarshalJSON() ([]byte, error) {
	intermediate := make(map[string]interface{})
	for k, v := range c.UnknownFields {
		intermediate[k] = v
	}
	if c.CassEnvSh != nil {
		intermediate["cassandra-env-sh"] = c.CassEnvSh
	}
	if c.CassandraYaml != nil {
		intermediate["cassandra-yaml"] = c.CassandraYaml
	}
	return json.Marshal(intermediate)
}

type cassEnvSh struct {
	AddtnlJVMOptions *[]string `json:"additional-jvm-opts,omitempty"`
	UnknownFields    map[string]interface{}
}

func (j *cassEnvSh) UnmarshalJSON(data []byte) error {
	intermediate := make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &intermediate); err != nil {
		return err
	}
	//
	if addtnlJVMOptsUnparsed, exists := intermediate["additional-jvm-opts"]; exists {
		parsedAddtnlJVMOpts := []string{} // Handle known additional opts string slice.
		err := json.Unmarshal(addtnlJVMOptsUnparsed, &parsedAddtnlJVMOpts)
		if err != nil {
			return err
		}
		j.AddtnlJVMOptions = &parsedAddtnlJVMOpts
		delete(intermediate, "additional-jvm-opts")
	}
	// Now parse the remaining fields as a map[string]interface{}.
	unknownFields := make(map[string]interface{})
	for k, v := range intermediate {
		var tmp interface{}
		if err := json.Unmarshal(v, &tmp); err != nil {
			return err
		}
		unknownFields[k] = tmp
	}
	j.UnknownFields = unknownFields
	return nil
}

// We just need this for flattening everything back down.
func (c cassEnvSh) MarshalJSON() ([]byte, error) {
	intermediate := make(map[string]interface{})
	for k, v := range c.UnknownFields {
		intermediate[k] = v
	}
	if c.AddtnlJVMOptions != nil && len(*c.AddtnlJVMOptions) > 0 {
		intermediate["additional-jvm-opts"] = c.AddtnlJVMOptions
	}
	return json.Marshal(intermediate)
}
