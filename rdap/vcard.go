// OpenRDAP
// Copyright 2017 Tom Harwood
// MIT License, see the LICENSE file.

package rdap

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// VCard represents a vCard.
//
// A vCard represents information about an individual or entity. It can include
// a name, telephone number, e-mail, delivery address, and other information.
//
// There are several vCard text formats. This implementation encodes/decodes the
// jCard format used by RDAP, as defined in https://tools.ietf.org/html/rfc7095.
//
// A jCard consists of an array of properties (e.g. "fn", "tel") describing the
// individual or entity. Properties may be repeated, e.g. to represent multiple
// telephone numbers. RFC6350 documents a set of standard properties.
//
// RFC7095 describes the JSON document format, which looks like:
//   ["vcard", [
//     [
//       ["version", {}, "text", "4.0"],
//       ["fn", {}, "text", "Joe Appleseed"],
//       ["tel", {
//             "type":["work", "voice"],
//           },
//           "uri",
//           "tel:+1-555-555-1234;ext=555"
//       ],
//       ...
//     ]
//   ]
type VCard struct {
	Properties []*VCardProperty
}

// VCardProperty represents a single vCard property.
//
// Each vCard property has four fields, these are:
//    Name   Parameters                  Type   Value
//    -----  --------------------------  -----  -----------------------------
//   ["tel", {"type":["work", "voice"]}, "uri", "tel:+1-555-555-1234;ext=555"]
type VCardProperty struct {
	Name string

	// vCard parameters can be a string, or array of strings.
	//
	// To simplify our usage, single strings are represented as an array of
	// length one.
	Parameters map[string][]string
	Type       string

	// A property value can be a simple type (string/float64/bool/nil), or be
	// an array. Arrays can be nested, and can contain a mixture of types.
	//
	// Value is one of the following:
	//   * string
	//   * float64
	//   * bool
	//   * nil
	//   * []interface{}. Can contain a mixture of these five types.
	//
	// To retrieve the property value flattened into a []string, use Values().
	Value interface{}
}

// Values returns a simplified representation of the VCardProperty value.
//
// This is convenient for accessing simple unstructured data (e.g. "fn", "tel").
//
// The simplified []string representation is created by flattening the
// (potentially nested) VCardProperty value, and converting all values to strings.
func (p *VCardProperty) Values() []string {
	strings := make([]string, 0, 1)

	p.appendValueStrings(p.Value, &strings)

	return strings
}

func (p *VCardProperty) appendValueStrings(v interface{}, strings *[]string) {
	switch v := v.(type) {
	case nil:
		*strings = append(*strings, "")
	case bool:
		*strings = append(*strings, strconv.FormatBool(v))
	case float64:
		*strings = append(*strings, strconv.FormatFloat(v, 'f', -1, 64))
	case string:
		*strings = append(*strings, v)
	case []interface{}:
		for _, v2 := range v {
			p.appendValueStrings(v2, strings)
		}
	default:
		panic("Unknown type")
	}

}

// String returns the vCard as a multiline human readable string. For example:
//
//   vCard[
//     version (type=text, parameters=map[]): [4.0]
//     mixed (type=text, parameters=map[]): [abc true 42 <nil> [def false 43]]
//   ]
//
// This is intended for debugging only, and is not machine parsable.
func (j *VCard) String() string {
	s := make([]string, 0, len(j.Properties))

	for _, s2 := range j.Properties {
		s = append(s, s2.String())
	}

	return "vCard[\n" + strings.Join(s, "\n") + "\n]"
}

// String returns the VCardProperty as a human readable string. For example:
//
//     mixed (type=text, parameters=map[]): [abc true 42 <nil> [def false 43]]
//
// This is intended for debugging only, and is not machine parsable.
func (p *VCardProperty) String() string {
	return fmt.Sprintf("  %s (type=%s, parameters=%v): %v", p.Name, p.Type, p.Parameters, p.Value)
}

// NewVCard creates a VCard from jsonDocument.
func NewVCard(jsonDocument []byte) (*VCard, error) {
	var top []interface{}
	err := json.Unmarshal(jsonDocument, &top)

	if err != nil {
		return nil, err
	}

	var vcard *VCard
	vcard, err = newVCardImpl(top)

	return vcard, err
}

func newVCardImpl(src interface{}) (*VCard, error) {
	top, ok := src.([]interface{})

	if !ok || len(top) != 2 {
		return nil, vCardError("structure is not a jCard (expected len=2 top level array)")
	} else if s, ok := top[0].(string); !(ok && s == "vcard") {
		return nil, vCardError("structure is not a jCard (missing 'vcard')")
	}

	var properties []interface{}

	properties, ok = top[1].([]interface{})
	if !ok {
		return nil, vCardError("structure is not a jCard (bad properties array)")
	}

	j := &VCard{
		Properties: make([]*VCardProperty, 0, len(properties)),
	}

	var p interface{}
	for _, p = range top[1].([]interface{}) {
		var a []interface{}
		var ok bool
		a, ok = p.([]interface{})

		if !ok {
			return nil, vCardError("jCard property was not an array")
		} else if len(a) < 4 {
			return nil, vCardError("jCard property too short (>=4 array elements required)")
		}

		name, ok := a[0].(string)

		if !ok {
			return nil, vCardError("jCard property name invalid")
		}

		var parameters map[string][]string
		var err error
		parameters, err = readParameters(a[1])

		if err != nil {
			return nil, err
		}

		propertyType, ok := a[2].(string)

		if !ok {
			return nil, vCardError("jCard property type invalid")
		}

		var value interface{}
		if len(a) == 4 {
			value, err = readValue(a[3], 0)
		} else {
			value, err = readValue(a[3:], 0)
		}

		if err != nil {
			return nil, err
		}

		property := &VCardProperty{
			Name:       name,
			Type:       propertyType,
			Parameters: parameters,
			Value:      value,
		}

		j.Properties = append(j.Properties, property)
	}

	return j, nil
}

// Get returns a list of the jCard Properties with VCardProperty name |name|.
func (j *VCard) Get(name string) []*VCardProperty {
	var properties []*VCardProperty

	for _, p := range j.Properties {
		if p.Name == name {
			properties = append(properties, p)
		}
	}

	return properties
}

func vCardError(e string) error {
	return fmt.Errorf("jCard error: %s", e)
}

func readParameters(p interface{}) (map[string][]string, error) {
	params := map[string][]string{}

	if _, ok := p.(map[string]interface{}); !ok {
		return nil, vCardError("jCard parameters invalid")
	}

	for k, v := range p.(map[string]interface{}) {
		if s, ok := v.(string); ok {
			params[k] = append(params[k], s)
		} else if arr, ok := v.([]interface{}); ok {
			for _, value := range arr {
				if s, ok := value.(string); ok {
					params[k] = append(params[k], s)
				}
			}
		}
	}

	return params, nil
}

func readValue(value interface{}, depth int) (interface{}, error) {
	switch value := value.(type) {
	case nil:
		return nil, nil
	case string:
		return value, nil
	case bool:
		return value, nil
	case float64:
		return value, nil
	case []interface{}:
		if depth == 3 {
			return "", vCardError("Structured value too deep")
		}

		result := make([]interface{}, 0, len(value))

		for _, v2 := range value {
			v3, err := readValue(v2, depth+1)

			if err != nil {
				return nil, err
			}

			result = append(result, v3)
		}

		return result, nil
	default:
		return nil, vCardError("Unknown JSON datatype in jCard value")
	}
}
