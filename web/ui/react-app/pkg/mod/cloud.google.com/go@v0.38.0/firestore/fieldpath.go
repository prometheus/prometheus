// Copyright 2017 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package firestore

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"

	"cloud.google.com/go/internal/fields"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

// A FieldPath is a non-empty sequence of non-empty fields that reference a value.
//
// A FieldPath value should only be necessary if one of the field names contains
// one of the runes ".˜*/[]". Most methods accept a simpler form of field path
// as a string in which the individual fields are separated by dots.
// For example,
//   []string{"a", "b"}
// is equivalent to the string form
//   "a.b"
// but
//   []string{"*"}
// has no equivalent string form.
type FieldPath []string

// parseDotSeparatedString constructs a FieldPath from a string that separates
// path components with dots. Other than splitting at dots and checking for invalid
// characters, it ignores everything else about the string,
// including attempts to quote field path compontents. So "a.`b.c`.d" is parsed into
// four parts, "a", "`b", "c`" and "d".
func parseDotSeparatedString(s string) (FieldPath, error) {
	const invalidRunes = "~*/[]"
	if strings.ContainsAny(s, invalidRunes) {
		return nil, fmt.Errorf("firestore: %q contains an invalid rune (one of %s)", s, invalidRunes)
	}
	fp := FieldPath(strings.Split(s, "."))
	if err := fp.validate(); err != nil {
		return nil, err
	}
	return fp, nil
}

func (fp1 FieldPath) equal(fp2 FieldPath) bool {
	if len(fp1) != len(fp2) {
		return false
	}
	for i, c1 := range fp1 {
		if c1 != fp2[i] {
			return false
		}
	}
	return true
}

func (fp1 FieldPath) prefixOf(fp2 FieldPath) bool {
	return len(fp1) <= len(fp2) && fp1.equal(fp2[:len(fp1)])
}

// Lexicographic ordering.
func (fp1 FieldPath) less(fp2 FieldPath) bool {
	for i := range fp1 {
		switch {
		case i >= len(fp2):
			return false
		case fp1[i] < fp2[i]:
			return true
		case fp1[i] > fp2[i]:
			return false
		}
	}
	// fp1 and fp2 are equal up to len(fp1).
	return len(fp1) < len(fp2)
}

// validate checks the validity of fp and returns an error if it is invalid.
func (fp FieldPath) validate() error {
	if len(fp) == 0 {
		return errors.New("firestore: empty field path")
	}
	for _, c := range fp {
		if len(c) == 0 {
			return errors.New("firestore: empty component in field path")
		}
	}
	return nil
}

// with creates a new FieldPath consisting of fp followed by k.
func (fp FieldPath) with(k string) FieldPath {
	r := make(FieldPath, len(fp), len(fp)+1)
	copy(r, fp)
	return append(r, k)
}

// checkNoDupOrPrefix checks whether any FieldPath is a prefix of (or equal to)
// another.
// It modifies the order of FieldPaths in its argument (via sorting).
func checkNoDupOrPrefix(fps []FieldPath) error {
	// Sort fps lexicographically.
	sort.Sort(byPath(fps))
	// Check adjacent pairs for prefix.
	for i := 1; i < len(fps); i++ {
		if fps[i-1].prefixOf(fps[i]) {
			return fmt.Errorf("field path %v cannot be used in the same update as %v", fps[i-1], fps[i])
		}
	}
	return nil
}

type byPath []FieldPath

func (b byPath) Len() int           { return len(b) }
func (b byPath) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byPath) Less(i, j int) bool { return b[i].less(b[j]) }

// setAtPath sets val at the location in m specified by fp, creating sub-maps as
// needed. m must not be nil. fp is assumed to be valid.
func setAtPath(m map[string]*pb.Value, fp FieldPath, val *pb.Value) {
	if val == nil {
		return
	}
	if len(fp) == 1 {
		m[fp[0]] = val
	} else {
		v, ok := m[fp[0]]
		if !ok {
			v = &pb.Value{ValueType: &pb.Value_MapValue{&pb.MapValue{Fields: map[string]*pb.Value{}}}}
			m[fp[0]] = v
		}
		// The type assertion below cannot fail, because setAtPath is only called
		// with either an empty map or one filled by setAtPath itself, and the
		// set of FieldPaths it is called with has been checked to make sure that
		// no path is the prefix of any other.
		setAtPath(v.GetMapValue().Fields, fp[1:], val)
	}
}

// getAtPath gets the value in data referred to by fp. The data argument can
// be a map or a struct.
// Compare with valueAtPath, which does the same thing for a document.
func getAtPath(v reflect.Value, fp FieldPath) (interface{}, error) {
	var err error
	for _, k := range fp {
		v, err = getAtField(v, k)
		if err != nil {
			return nil, err
		}
	}
	return v.Interface(), nil
}

// getAtField returns the equivalent of v[k], if v is a map, or v.k if v is a struct.
func getAtField(v reflect.Value, k string) (reflect.Value, error) {
	switch v.Kind() {
	case reflect.Map:
		if r := v.MapIndex(reflect.ValueOf(k)); r.IsValid() {
			return r, nil
		}

	case reflect.Struct:
		fm, err := fieldMap(v.Type())
		if err != nil {
			return reflect.Value{}, err
		}
		if f, ok := fm[k]; ok {
			return v.FieldByIndex(f.Index), nil
		}

	case reflect.Interface:
		return getAtField(v.Elem(), k)

	case reflect.Ptr:
		return getAtField(v.Elem(), k)
	}
	return reflect.Value{}, fmt.Errorf("firestore: no field %q for value %#v", k, v)
}

// fieldMapCache holds maps from from Firestore field name to struct field,
// keyed by struct type.
var fieldMapCache sync.Map

func fieldMap(t reflect.Type) (map[string]fields.Field, error) {
	x, ok := fieldMapCache.Load(t)
	if !ok {
		fieldList, err := fieldCache.Fields(t)
		if err != nil {
			x = err
		} else {
			m := map[string]fields.Field{}
			for _, f := range fieldList {
				m[f.Name] = f
			}
			x = m
		}
		fieldMapCache.Store(t, x)
	}
	if err, ok := x.(error); ok {
		return nil, err
	}
	return x.(map[string]fields.Field), nil
}

// toServiceFieldPath converts fp the form required by the Firestore service.
// It assumes fp has been validated.
func (fp FieldPath) toServiceFieldPath() string {
	cs := make([]string, len(fp))
	for i, c := range fp {
		cs[i] = toServiceFieldPathComponent(c)
	}
	return strings.Join(cs, ".")
}

func toServiceFieldPaths(fps []FieldPath) []string {
	var sfps []string
	for _, fp := range fps {
		sfps = append(sfps, fp.toServiceFieldPath())
	}
	return sfps
}

// Google SQL syntax for an unquoted field.
var unquotedFieldRegexp = regexp.MustCompile("^[A-Za-z_][A-Za-z_0-9]*$")

// toServiceFieldPathComponent returns a string that represents key and is a valid
// field path component.
func toServiceFieldPathComponent(key string) string {
	if unquotedFieldRegexp.MatchString(key) {
		return key
	}
	var buf bytes.Buffer
	buf.WriteRune('`')
	for _, r := range key {
		if r == '`' || r == '\\' {
			buf.WriteRune('\\')
		}
		buf.WriteRune(r)
	}
	buf.WriteRune('`')
	return buf.String()
}
