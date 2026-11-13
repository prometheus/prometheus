// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package yamlutil provides YAML utility functions.
package yamlutil

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"

	"go.yaml.in/yaml/v4"
)

// Unmarshal is like yaml.Unmarshal, but tolerates empty input, for which it
// returns no error.
func Unmarshal(in []byte, out any) error {
	return decode(in, out, false)
}

// UnmarshalStrict is like yaml.Unmarshal but returns an error if there are
// keys in the data that do not have corresponding struct members. Like
// Unmarshal, it tolerates empty input.
func UnmarshalStrict(in []byte, out any) error {
	return decode(in, out, true)
}

func decode(in []byte, out any, knownFields bool) error {
	dec := yaml.NewDecoder(bytes.NewReader(in))
	dec.KnownFields(knownFields)
	err := dec.Decode(out)
	// yaml/v4 returns a bare io.EOF for empty input, but for compatibility with
	// v2 behavior we treat this as success (no error).
	//
	// This must compare with == rather than errors.Is: an error returned by a
	// custom UnmarshalYAML that happens to wrap io.EOF is reported by the decoder
	// as a type error, and errors.Is would match it and discard the failure.
	if err == io.EOF { //nolint:errorlint // Deliberately an identity check, see above.
		return nil
	}
	return err
}

// IsTypeError reports whether err was caused by one or more values that could
// not be decoded into the target Go types, e.g. unknown keys when unmarshalling
// strictly.
func IsTypeError(err error) bool {
	_, ok := errors.AsType[*yaml.LoadErrors](err)
	return ok
}

// ReplaceTypeNames replaces oldTyp with newTyp in the message of the type
// error contained in err. It is meant for unmarshalling into dynamically
// constructed types, whose names would otherwise leak into error messages.
//
// The messages of the type error are rewritten in place, so that the yaml
// library keeps merging nested errors. If err has already been formatted by a
// wrapper such as fmt.Errorf, the reported message is rewritten instead, which
// is why the returned error may not be err itself. It unwraps to err, though.
func ReplaceTypeNames(err error, oldTyp, newTyp reflect.Type) error {
	if err == nil || !IsTypeError(err) {
		return err
	}
	oldStr, newStr := oldTyp.String(), newTyp.String()

	if le, ok := errors.AsType[*yaml.LoadErrors](err); ok {
		for _, e := range le.Errors {
			e.Message = strings.ReplaceAll(e.Message, oldStr, newStr)
		}
	}

	if msg := err.Error(); strings.Contains(msg, oldStr) {
		return &typeNameError{err: err, msg: strings.ReplaceAll(msg, oldStr, newStr)}
	}
	return err
}

// typeNameError is an error whose message differs from the message of the error
// it wraps. Rewriting the message in place does not work, because a wrapper
// such as fmt.Errorf has already formatted it.
type typeNameError struct {
	err error
	msg string
}

// Error implements the error interface.
func (e *typeNameError) Error() string { return e.msg }

// Unwrap returns the wrapped error, so that errors.Is and errors.As keep
// working.
func (e *typeNameError) Unwrap() error { return e.err }

// Marshal marshals v into YAML with 2-space indentation.
//
// It deliberately uses an Encoder rather than yaml.Marshal: the latter disables
// line wrapping (width -1), while yaml/v2 wrapped long plain scalars at 80
// columns. Keeping the wrapping avoids changing the emitted values that
// Prometheus prints, see TestMarshalWrapping.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
