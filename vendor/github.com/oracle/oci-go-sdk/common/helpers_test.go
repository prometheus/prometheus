// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.

package common

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestStructToString(t *testing.T) {
	ints := []int{1, 2, 4}

	s := struct {
		Anint        *int
		AString      *string
		AFloat       *float32
		SimpleString string
		SimepleInt   int
		IntSlice     *[]int
	}{Int(1),
		String("one"),
		Float32(2.3),
		"simple",
		2, &ints}

	str := PointerString(s)
	assert.Contains(t, str, "one")
	assert.Contains(t, str, "1")
	assert.Contains(t, str, "[1 2 4]")
}

type sample struct {
	Anint        *int
	AString      *string
	AFloat       *float32
	SimpleString string
	SimepleInt   int
	Nested       sampleNested
}

func (s sample) String() string {
	str := PointerString(s)
	return str
}

type sampleNested struct {
	NestedInt    *int
	NestedBool   *bool
	NestedString *string
	Thestring    string
}

func (s sampleNested) String() string {
	str := PointerString(s)
	return str
}

func TestStructToString_Nested(t *testing.T) {
	s := sample{Anint: Int(1),
		AString:      nil,
		AFloat:       Float32(2.3),
		SimpleString: "simple",
		SimepleInt:   2,
	}
	s.Nested.NestedBool = Bool(true)
	s.Nested.NestedString = nil
	s.Nested.Thestring = "somestring"
	s.Nested.NestedInt = Int(2)

	str := fmt.Sprintf("%s", s)
	assert.Contains(t, str, "1")
	assert.Contains(t, str, "somestring")
	assert.Contains(t, str, "<nil>")
}

func TestDateParsing_LastModifiedHeaderDate(t *testing.T) {
	data := []string{"Tue, 2 Jan 2018 17:49:29 GMT", "Tue, 02 Jan 2018 17:49:29 GMT"}
	for _, val := range data {
		tt, err := tryParsingTimeWithValidFormatsForHeaders([]byte(val), "lastmodified")
		assert.NoError(t, err)
		assert.Equal(t, tt.Day(), 2)
	}
}
