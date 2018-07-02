// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.

package common

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// String returns a pointer to the provided string
func String(value string) *string {
	return &value
}

// Int returns a pointer to the provided int
func Int(value int) *int {
	return &value
}

// Uint returns a pointer to the provided uint
func Uint(value uint) *uint {
	return &value
}

//Float32 returns a pointer to the provided float32
func Float32(value float32) *float32 {
	return &value
}

//Float64 returns a pointer to the provided float64
func Float64(value float64) *float64 {
	return &value
}

//Bool returns a pointer to the provided bool
func Bool(value bool) *bool {
	return &value
}

//PointerString prints the values of pointers in a struct
//Producing a human friendly string for an struct with pointers.
//useful when debugging the values of a struct
func PointerString(datastruct interface{}) (representation string) {
	val := reflect.ValueOf(datastruct)
	typ := reflect.TypeOf(datastruct)
	all := make([]string, 2)
	all = append(all, "{")
	for i := 0; i < typ.NumField(); i++ {
		sf := typ.Field(i)

		//unexported
		if sf.PkgPath != "" && !sf.Anonymous {
			continue
		}

		sv := val.Field(i)
		stringValue := ""
		if isNil(sv) {
			stringValue = fmt.Sprintf("%s=<nil>", sf.Name)
		} else {
			if sv.Type().Kind() == reflect.Ptr {
				sv = sv.Elem()
			}
			stringValue = fmt.Sprintf("%s=%v", sf.Name, sv)
		}
		all = append(all, stringValue)
	}
	all = append(all, "}")
	representation = strings.TrimSpace(strings.Join(all, " "))
	return
}

// SDKTime a time struct, which renders to/from json using RFC339
type SDKTime struct {
	time.Time
}

func sdkTimeFromTime(t time.Time) SDKTime {
	return SDKTime{t}
}

func now() *SDKTime {
	t := SDKTime{time.Now()}
	return &t
}

var timeType = reflect.TypeOf(SDKTime{})
var timeTypePtr = reflect.TypeOf(&SDKTime{})

const sdkTimeFormat = time.RFC3339

const rfc1123OptionalLeadingDigitsInDay = "Mon, _2 Jan 2006 15:04:05 MST"

func formatTime(t SDKTime) string {
	return t.Format(sdkTimeFormat)
}

func tryParsingTimeWithValidFormatsForHeaders(data []byte, headerName string) (t time.Time, err error) {
	header := strings.ToLower(headerName)
	switch header {
	case "lastmodified", "date":
		t, err = tryParsing(data, time.RFC3339, time.RFC1123, rfc1123OptionalLeadingDigitsInDay, time.RFC850, time.ANSIC)
		return
	default: //By default we parse with RFC3339
		t, err = time.Parse(sdkTimeFormat, string(data))
		return
	}
}

func tryParsing(data []byte, layouts ...string) (tm time.Time, err error) {
	datestring := string(data)
	for _, l := range layouts {
		tm, err = time.Parse(l, datestring)
		if err == nil {
			return
		}
	}
	err = fmt.Errorf("Could not parse time: %s with formats: %s", datestring, layouts[:])
	return
}

// UnmarshalJSON unmarshals from json
func (t *SDKTime) UnmarshalJSON(data []byte) (e error) {
	s := string(data)
	if s == "null" {
		t.Time = time.Time{}
	} else {
		//Try parsing with RFC3339
		t.Time, e = time.Parse(`"`+sdkTimeFormat+`"`, string(data))
	}
	return
}

// MarshalJSON marshals to JSON
func (t *SDKTime) MarshalJSON() (buff []byte, e error) {
	s := t.Format(sdkTimeFormat)
	buff = []byte(`"` + s + `"`)
	return
}

// PrivateKeyFromBytes is a helper function that will produce a RSA private
// key from bytes.
func PrivateKeyFromBytes(pemData []byte, password *string) (key *rsa.PrivateKey, e error) {
	if pemBlock, _ := pem.Decode(pemData); pemBlock != nil {
		decrypted := pemBlock.Bytes
		if x509.IsEncryptedPEMBlock(pemBlock) {
			if password == nil {
				e = fmt.Errorf("private_key_password is required for encrypted private keys")
				return
			}
			if decrypted, e = x509.DecryptPEMBlock(pemBlock, []byte(*password)); e != nil {
				return
			}
		}

		key, e = x509.ParsePKCS1PrivateKey(decrypted)

	} else {
		e = fmt.Errorf("PEM data was not found in buffer")
		return
	}
	return
}

func generateRandUUID() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	uuid := fmt.Sprintf("%x%x%x%x%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])

	return uuid, nil
}
