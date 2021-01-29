// Copyright 2014 Unknwon
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package ini

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"
)

// NameMapper represents a ini tag name mapper.
type NameMapper func(string) string

// Built-in name getters.
var (
	// AllCapsUnderscore converts to format ALL_CAPS_UNDERSCORE.
	AllCapsUnderscore NameMapper = func(raw string) string {
		newstr := make([]rune, 0, len(raw))
		for i, chr := range raw {
			if isUpper := 'A' <= chr && chr <= 'Z'; isUpper {
				if i > 0 {
					newstr = append(newstr, '_')
				}
			}
			newstr = append(newstr, unicode.ToUpper(chr))
		}
		return string(newstr)
	}
	// TitleUnderscore converts to format title_underscore.
	TitleUnderscore NameMapper = func(raw string) string {
		newstr := make([]rune, 0, len(raw))
		for i, chr := range raw {
			if isUpper := 'A' <= chr && chr <= 'Z'; isUpper {
				if i > 0 {
					newstr = append(newstr, '_')
				}
				chr -= ('A' - 'a')
			}
			newstr = append(newstr, chr)
		}
		return string(newstr)
	}
)

func (s *Section) parseFieldName(raw, actual string) string {
	if len(actual) > 0 {
		return actual
	}
	if s.f.NameMapper != nil {
		return s.f.NameMapper(raw)
	}
	return raw
}

func parseDelim(actual string) string {
	if len(actual) > 0 {
		return actual
	}
	return ","
}

var reflectTime = reflect.TypeOf(time.Now()).Kind()

// setSliceWithProperType sets proper values to slice based on its type.
func setSliceWithProperType(key *Key, field reflect.Value, delim string) error {
	strs := key.Strings(delim)
	numVals := len(strs)
	if numVals == 0 {
		return nil
	}

	var vals interface{}

	sliceOf := field.Type().Elem().Kind()
	switch sliceOf {
	case reflect.String:
		vals = strs
	case reflect.Int:
		vals = key.Ints(delim)
	case reflect.Int64:
		vals = key.Int64s(delim)
	case reflect.Uint:
		vals = key.Uints(delim)
	case reflect.Uint64:
		vals = key.Uint64s(delim)
	case reflect.Float64:
		vals = key.Float64s(delim)
	case reflectTime:
		vals = key.Times(delim)
	default:
		return fmt.Errorf("unsupported type '[]%s'", sliceOf)
	}

	slice := reflect.MakeSlice(field.Type(), numVals, numVals)
	for i := 0; i < numVals; i++ {
		switch sliceOf {
		case reflect.String:
			slice.Index(i).Set(reflect.ValueOf(vals.([]string)[i]))
		case reflect.Int:
			slice.Index(i).Set(reflect.ValueOf(vals.([]int)[i]))
		case reflect.Int64:
			slice.Index(i).Set(reflect.ValueOf(vals.([]int64)[i]))
		case reflect.Uint:
			slice.Index(i).Set(reflect.ValueOf(vals.([]uint)[i]))
		case reflect.Uint64:
			slice.Index(i).Set(reflect.ValueOf(vals.([]uint64)[i]))
		case reflect.Float64:
			slice.Index(i).Set(reflect.ValueOf(vals.([]float64)[i]))
		case reflectTime:
			slice.Index(i).Set(reflect.ValueOf(vals.([]time.Time)[i]))
		}
	}
	field.Set(slice)
	return nil
}

// setWithProperType sets proper value to field based on its type,
// but it does not return error for failing parsing,
// because we want to use default value that is already assigned to strcut.
func setWithProperType(t reflect.Type, key *Key, field reflect.Value, delim string) error {
	switch t.Kind() {
	case reflect.String:
		if len(key.String()) == 0 {
			return nil
		}
		field.SetString(key.String())
	case reflect.Bool:
		boolVal, err := key.Bool()
		if err != nil {
			return nil
		}
		field.SetBool(boolVal)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		durationVal, err := key.Duration()
		// Skip zero value
		if err == nil && int(durationVal) > 0 {
			field.Set(reflect.ValueOf(durationVal))
			return nil
		}

		intVal, err := key.Int64()
		if err != nil || intVal == 0 {
			return nil
		}
		field.SetInt(intVal)
	//	byte is an alias for uint8, so supporting uint8 breaks support for byte
	case reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		durationVal, err := key.Duration()
		// Skip zero value
		if err == nil && int(durationVal) > 0 {
			field.Set(reflect.ValueOf(durationVal))
			return nil
		}

		uintVal, err := key.Uint64()
		if err != nil {
			return nil
		}
		field.SetUint(uintVal)

	case reflect.Float64:
		floatVal, err := key.Float64()
		if err != nil {
			return nil
		}
		field.SetFloat(floatVal)
	case reflectTime:
		timeVal, err := key.Time()
		if err != nil {
			return nil
		}
		field.Set(reflect.ValueOf(timeVal))
	case reflect.Slice:
		return setSliceWithProperType(key, field, delim)
	default:
		return fmt.Errorf("unsupported type '%s'", t)
	}
	return nil
}

func (s *Section) mapTo(val reflect.Value) error {
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := val.Field(i)
		tpField := typ.Field(i)

		tag := tpField.Tag.Get("ini")
		if tag == "-" {
			continue
		}

		opts := strings.SplitN(tag, ",", 2) // strip off possible omitempty
		fieldName := s.parseFieldName(tpField.Name, opts[0])
		if len(fieldName) == 0 || !field.CanSet() {
			continue
		}

		isAnonymous := tpField.Type.Kind() == reflect.Ptr && tpField.Anonymous
		isStruct := tpField.Type.Kind() == reflect.Struct
		if isAnonymous {
			field.Set(reflect.New(tpField.Type.Elem()))
		}

		if isAnonymous || isStruct {
			if sec, err := s.f.GetSection(fieldName); err == nil {
				if err = sec.mapTo(field); err != nil {
					return fmt.Errorf("error mapping field(%s): %v", fieldName, err)
				}
				continue
			}
		}

		if key, err := s.GetKey(fieldName); err == nil {
			if err = setWithProperType(tpField.Type, key, field, parseDelim(tpField.Tag.Get("delim"))); err != nil {
				return fmt.Errorf("error mapping field(%s): %v", fieldName, err)
			}
		}
	}
	return nil
}

// MapTo maps section to given struct.
func (s *Section) MapTo(v interface{}) error {
	typ := reflect.TypeOf(v)
	val := reflect.ValueOf(v)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
		val = val.Elem()
	} else {
		return errors.New("cannot map to non-pointer struct")
	}

	return s.mapTo(val)
}

// MapTo maps file to given struct.
func (f *File) MapTo(v interface{}) error {
	return f.Section("").MapTo(v)
}

// MapTo maps data sources to given struct with name mapper.
func MapToWithMapper(v interface{}, mapper NameMapper, source interface{}, others ...interface{}) error {
	cfg, err := Load(source, others...)
	if err != nil {
		return err
	}
	cfg.NameMapper = mapper
	return cfg.MapTo(v)
}

// MapTo maps data sources to given struct.
func MapTo(v, source interface{}, others ...interface{}) error {
	return MapToWithMapper(v, nil, source, others...)
}

// reflectSliceWithProperType does the opposite thing as setSliceWithProperType.
func reflectSliceWithProperType(key *Key, field reflect.Value, delim string) error {
	slice := field.Slice(0, field.Len())
	if field.Len() == 0 {
		return nil
	}

	var buf bytes.Buffer
	sliceOf := field.Type().Elem().Kind()
	for i := 0; i < field.Len(); i++ {
		switch sliceOf {
		case reflect.String:
			buf.WriteString(slice.Index(i).String())
		case reflect.Int, reflect.Int64:
			buf.WriteString(fmt.Sprint(slice.Index(i).Int()))
		case reflect.Uint, reflect.Uint64:
			buf.WriteString(fmt.Sprint(slice.Index(i).Uint()))
		case reflect.Float64:
			buf.WriteString(fmt.Sprint(slice.Index(i).Float()))
		case reflectTime:
			buf.WriteString(slice.Index(i).Interface().(time.Time).Format(time.RFC3339))
		default:
			return fmt.Errorf("unsupported type '[]%s'", sliceOf)
		}
		buf.WriteString(delim)
	}
	key.SetValue(buf.String()[:buf.Len()-1])
	return nil
}

// reflectWithProperType does the opposite thing as setWithProperType.
func reflectWithProperType(t reflect.Type, key *Key, field reflect.Value, delim string) error {
	switch t.Kind() {
	case reflect.String:
		key.SetValue(field.String())
	case reflect.Bool:
		key.SetValue(fmt.Sprint(field.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		key.SetValue(fmt.Sprint(field.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		key.SetValue(fmt.Sprint(field.Uint()))
	case reflect.Float32, reflect.Float64:
		key.SetValue(fmt.Sprint(field.Float()))
	case reflectTime:
		key.SetValue(fmt.Sprint(field.Interface().(time.Time).Format(time.RFC3339)))
	case reflect.Slice:
		return reflectSliceWithProperType(key, field, delim)
	default:
		return fmt.Errorf("unsupported type '%s'", t)
	}
	return nil
}

// CR: copied from encoding/json/encode.go with modifications of time.Time support.
// TODO: add more test coverage.
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflectTime:
		return v.Interface().(time.Time).IsZero()
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}

func (s *Section) reflectFrom(val reflect.Value) error {
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := val.Field(i)
		tpField := typ.Field(i)

		tag := tpField.Tag.Get("ini")
		if tag == "-" {
			continue
		}

		opts := strings.SplitN(tag, ",", 2)
		if len(opts) == 2 && opts[1] == "omitempty" && isEmptyValue(field) {
			continue
		}

		fieldName := s.parseFieldName(tpField.Name, opts[0])
		if len(fieldName) == 0 || !field.CanSet() {
			continue
		}

		if (tpField.Type.Kind() == reflect.Ptr && tpField.Anonymous) ||
			(tpField.Type.Kind() == reflect.Struct && tpField.Type.Name() != "Time") {
			// Note: The only error here is section doesn't exist.
			sec, err := s.f.GetSection(fieldName)
			if err != nil {
				// Note: fieldName can never be empty here, ignore error.
				sec, _ = s.f.NewSection(fieldName)
			}
			if err = sec.reflectFrom(field); err != nil {
				return fmt.Errorf("error reflecting field (%s): %v", fieldName, err)
			}
			continue
		}

		// Note: Same reason as secion.
		key, err := s.GetKey(fieldName)
		if err != nil {
			key, _ = s.NewKey(fieldName, "")
		}
		if err = reflectWithProperType(tpField.Type, key, field, parseDelim(tpField.Tag.Get("delim"))); err != nil {
			return fmt.Errorf("error reflecting field (%s): %v", fieldName, err)
		}

	}
	return nil
}

// ReflectFrom reflects secion from given struct.
func (s *Section) ReflectFrom(v interface{}) error {
	typ := reflect.TypeOf(v)
	val := reflect.ValueOf(v)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
		val = val.Elem()
	} else {
		return errors.New("cannot reflect from non-pointer struct")
	}

	return s.reflectFrom(val)
}

// ReflectFrom reflects file from given struct.
func (f *File) ReflectFrom(v interface{}) error {
	return f.Section("").ReflectFrom(v)
}

// ReflectFrom reflects data sources from given struct with name mapper.
func ReflectFromWithMapper(cfg *File, v interface{}, mapper NameMapper) error {
	cfg.NameMapper = mapper
	return cfg.ReflectFrom(v)
}

// ReflectFrom reflects data sources from given struct.
func ReflectFrom(cfg *File, v interface{}) error {
	return ReflectFromWithMapper(cfg, v, nil)
}
