package xmlrpc

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Base64 represents value in base64 encoding
type Base64 string

type encodeFunc func(reflect.Value) ([]byte, error)

func marshal(v interface{}) ([]byte, error) {
	if v == nil {
		return []byte{}, nil
	}

	val := reflect.ValueOf(v)
	return encodeValue(val)
}

func encodeValue(val reflect.Value) ([]byte, error) {
	var b []byte
	var err error

	if val.Kind() == reflect.Ptr || val.Kind() == reflect.Interface {
		if val.IsNil() {
			return []byte("<value/>"), nil
		}

		val = val.Elem()
	}

	switch val.Kind() {
	case reflect.Struct:
		switch val.Interface().(type) {
		case time.Time:
			t := val.Interface().(time.Time)
			b = []byte(fmt.Sprintf("<dateTime.iso8601>%s</dateTime.iso8601>", t.Format(iso8601)))
		default:
			b, err = encodeStruct(val)
		}
	case reflect.Map:
		b, err = encodeMap(val)
	case reflect.Slice:
		b, err = encodeSlice(val)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b = []byte(fmt.Sprintf("<int>%s</int>", strconv.FormatInt(val.Int(), 10)))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		b = []byte(fmt.Sprintf("<i4>%s</i4>", strconv.FormatUint(val.Uint(), 10)))
	case reflect.Float32, reflect.Float64:
		b = []byte(fmt.Sprintf("<double>%s</double>",
			strconv.FormatFloat(val.Float(), 'f', -1, val.Type().Bits())))
	case reflect.Bool:
		if val.Bool() {
			b = []byte("<boolean>1</boolean>")
		} else {
			b = []byte("<boolean>0</boolean>")
		}
	case reflect.String:
		var buf bytes.Buffer

		xml.Escape(&buf, []byte(val.String()))

		if _, ok := val.Interface().(Base64); ok {
			b = []byte(fmt.Sprintf("<base64>%s</base64>", buf.String()))
		} else {
			b = []byte(fmt.Sprintf("<string>%s</string>", buf.String()))
		}
	default:
		return nil, fmt.Errorf("xmlrpc encode error: unsupported type")
	}

	if err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf("<value>%s</value>", string(b))), nil
}

func encodeStruct(structVal reflect.Value) ([]byte, error) {
	var b bytes.Buffer

	b.WriteString("<struct>")

	structType := structVal.Type()
	for i := 0; i < structType.NumField(); i++ {
		fieldVal := structVal.Field(i)
		fieldType := structType.Field(i)

		name := fieldType.Tag.Get("xmlrpc")
		// if the tag has the omitempty property, skip it
		if strings.HasSuffix(name, ",omitempty") && isZero(fieldVal) {
			continue
		}
		name = strings.TrimSuffix(name, ",omitempty")
		if name == "" {
			name = fieldType.Name
		}

		p, err := encodeValue(fieldVal)
		if err != nil {
			return nil, err
		}

		b.WriteString("<member>")
		b.WriteString(fmt.Sprintf("<name>%s</name>", name))
		b.Write(p)
		b.WriteString("</member>")
	}

	b.WriteString("</struct>")

	return b.Bytes(), nil
}

var sortMapKeys bool

func encodeMap(val reflect.Value) ([]byte, error) {
	var t = val.Type()

	if t.Key().Kind() != reflect.String {
		return nil, fmt.Errorf("xmlrpc encode error: only maps with string keys are supported")
	}

	var b bytes.Buffer

	b.WriteString("<struct>")

	keys := val.MapKeys()

	if sortMapKeys {
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	}

	for i := 0; i < val.Len(); i++ {
		key := keys[i]
		kval := val.MapIndex(key)

		b.WriteString("<member>")
		b.WriteString(fmt.Sprintf("<name>%s</name>", key.String()))

		p, err := encodeValue(kval)

		if err != nil {
			return nil, err
		}

		b.Write(p)
		b.WriteString("</member>")
	}

	b.WriteString("</struct>")

	return b.Bytes(), nil
}

func encodeSlice(val reflect.Value) ([]byte, error) {
	var b bytes.Buffer

	b.WriteString("<array><data>")

	for i := 0; i < val.Len(); i++ {
		p, err := encodeValue(val.Index(i))
		if err != nil {
			return nil, err
		}

		b.Write(p)
	}

	b.WriteString("</data></array>")

	return b.Bytes(), nil
}
