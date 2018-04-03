/*
Copyright 2014 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// IntOrString is a type that can hold an int or a string.  When used in
// JSON or YAML marshalling and unmarshalling, it produces or consumes the
// inner type.  This allows you to have, for example, a JSON field that can
// accept a name or number.
type IntOrString struct {
	Kind   IntstrKind
	IntVal int
	StrVal string
}

// IntstrKind represents the stored type of IntOrString.
type IntstrKind int

const (
	IntstrInt    IntstrKind = iota // The IntOrString holds an int.
	IntstrString                   // The IntOrString holds a string.
)

// UnmarshalJSON implements the json.Unmarshaller interface.
func (intstr *IntOrString) UnmarshalJSON(value []byte) error {
	if value[0] == '"' {
		intstr.Kind = IntstrString
		return json.Unmarshal(value, &intstr.StrVal)
	}
	intstr.Kind = IntstrInt
	return json.Unmarshal(value, &intstr.IntVal)
}

// String returns the string value, or Itoa's the int value.
func (intstr *IntOrString) String() string {
	if intstr.Kind == IntstrString {
		return intstr.StrVal
	}
	return strconv.Itoa(intstr.IntVal)
}

// MarshalJSON implements the json.Marshaller interface.
func (intstr IntOrString) MarshalJSON() ([]byte, error) {
	switch intstr.Kind {
	case IntstrInt:
		return json.Marshal(intstr.IntVal)
	case IntstrString:
		return json.Marshal(intstr.StrVal)
	default:
		return []byte{}, fmt.Errorf("impossible IntOrString.Kind")
	}
}

// UID is a type that holds unique ID values, including UUIDs.  Because we
// don't ONLY use UUIDs, this is an alias to string.  Being a type captures
// intent and helps make sure that UIDs and names do not get conflated.
type UID string
