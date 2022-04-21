// This file contains horrible things that allow us to marshal/unmarshal the parts of the config we care about to/from json, without losing the fields we don't
// care about.
package cdc

import "encoding/json"

type configData struct {
	JvmOptions    *jvmOptions `json:"jvm-options,omitempty"`
	UnknownFields map[string]interface{}
}

func (c *configData) UnmarshalJSON(data []byte) error {
	var intermediate = make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(data), &intermediate); err != nil {
		return err
	}
	// If jvm-options key exists, parse, add to c.JvmOptions field, delete from intermediate map.
	if jvmOptsUnparsed, exists := intermediate["jvm-options"]; exists {
		parsedjvmOpts := jvmOptions{} // First parse the known field "jvm-options" into the known struct jvmOptions{}
		if err := json.Unmarshal(jvmOptsUnparsed, &parsedjvmOpts); err != nil {
			return err
		}
		c.JvmOptions = &parsedjvmOpts
		delete(intermediate, "jvm-options")
	}
	// Now parse the remaining fields as a map[string]interface{}.
	var unknownFields = make(map[string]interface{})
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
	if c.JvmOptions != nil {
		intermediate["jvm-options"] = c.JvmOptions
	}
	return json.Marshal(intermediate)
}

type jvmOptions struct {
	AddtnlJVMOptions *[]string `json:"additional-jvm-opts,omitempty"`
	UnknownFields    map[string]interface{}
}

func (j *jvmOptions) UnmarshalJSON(data []byte) error {
	var intermediate = make(map[string]json.RawMessage)
	err := json.Unmarshal([]byte(data), &intermediate)
	if err != nil {
		return err
	}
	//
	if addtnlOptsUnparsed, exists := intermediate["additional-jvm-opts"]; exists {
		parsedAddtnlOpts := []string{} // Handle known additional opts string slice.
		err := json.Unmarshal(addtnlOptsUnparsed, &parsedAddtnlOpts)
		if err != nil {
			return err
		}
		j.AddtnlJVMOptions = &parsedAddtnlOpts
		delete(intermediate, "additional-jvm-opts")
	}
	// Now parse the remaining fields as a map[string]interface{}.
	var unknownFields = make(map[string]interface{})
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
func (c jvmOptions) MarshalJSON() ([]byte, error) {
	intermediate := make(map[string]interface{})
	for k, v := range c.UnknownFields {
		intermediate[k] = v
	}
	if c.AddtnlJVMOptions != nil && len(*c.AddtnlJVMOptions) > 0 {
		intermediate["additional-jvm-opts"] = c.AddtnlJVMOptions
	}
	return json.Marshal(intermediate)
}
