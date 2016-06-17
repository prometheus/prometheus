package autorest

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"strconv"
)

// EncodedAs is a series of constants specifying various data encodings
type EncodedAs string

const (
	// EncodedAsJSON states that data is encoded as JSON
	EncodedAsJSON EncodedAs = "JSON"

	// EncodedAsXML states that data is encoded as Xml
	EncodedAsXML EncodedAs = "XML"
)

// Decoder defines the decoding method json.Decoder and xml.Decoder share
type Decoder interface {
	Decode(v interface{}) error
}

// NewDecoder creates a new decoder appropriate to the passed encoding.
// encodedAs specifies the type of encoding and r supplies the io.Reader containing the
// encoded data.
func NewDecoder(encodedAs EncodedAs, r io.Reader) Decoder {
	if encodedAs == EncodedAsJSON {
		return json.NewDecoder(r)
	} else if encodedAs == EncodedAsXML {
		return xml.NewDecoder(r)
	}
	return nil
}

// CopyAndDecode decodes the data from the passed io.Reader while making a copy. Having a copy
// is especially useful if there is a chance the data will fail to decode.
// encodedAs specifies the expected encoding, r provides the io.Reader to the data, and v
// is the decoding destination.
func CopyAndDecode(encodedAs EncodedAs, r io.Reader, v interface{}) (bytes.Buffer, error) {
	b := bytes.Buffer{}
	return b, NewDecoder(encodedAs, io.TeeReader(r, &b)).Decode(v)
}

func readBool(r io.Reader) (bool, error) {
	s, err := readString(r)
	if err == nil {
		return strconv.ParseBool(s)
	}
	return false, err
}

func readFloat32(r io.Reader) (float32, error) {
	s, err := readString(r)
	if err == nil {
		v, err := strconv.ParseFloat(s, 32)
		return float32(v), err
	}
	return float32(0), err
}

func readFloat64(r io.Reader) (float64, error) {
	s, err := readString(r)
	if err == nil {
		v, err := strconv.ParseFloat(s, 64)
		return v, err
	}
	return float64(0), err
}

func readInt32(r io.Reader) (int32, error) {
	s, err := readString(r)
	if err == nil {
		v, err := strconv.ParseInt(s, 10, 32)
		return int32(v), err
	}
	return int32(0), err
}

func readInt64(r io.Reader) (int64, error) {
	s, err := readString(r)
	if err == nil {
		v, err := strconv.ParseInt(s, 10, 64)
		return v, err
	}
	return int64(0), err
}

func readString(r io.Reader) (string, error) {
	b, err := ioutil.ReadAll(r)
	return string(b), err
}

func containsInt(ints []int, n int) bool {
	for _, i := range ints {
		if i == n {
			return true
		}
	}
	return false
}

func escapeValueStrings(m map[string]string) map[string]string {
	for key, value := range m {
		m[key] = url.QueryEscape(value)
	}
	return m
}

func ensureValueStrings(mapOfInterface map[string]interface{}) map[string]string {
	mapOfStrings := make(map[string]string)
	for key, value := range mapOfInterface {
		mapOfStrings[key] = ensureValueString(value)
	}
	return mapOfStrings
}

func ensureValueString(value interface{}) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}
