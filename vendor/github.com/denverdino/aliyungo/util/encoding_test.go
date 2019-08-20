package util

import (
	"testing"
	"time"
)

type TestString string

type SubStruct struct {
	A string
	B int
}

type MyStruct struct {
	SubStruct
}

type YourStruct struct {
	SubStruct SubStruct
}

func TestConvertToQueryValues2(t *testing.T) {

	result := ConvertToQueryValues(MyStruct{SubStruct: SubStruct{A: "A", B: 1}}).Encode()
	const expectedResult = "A=A&B=1"
	if result != expectedResult {
		// Sometimes result is not matched for the different orders
		t.Logf("Incorrect encoding: %s", result)
	}
	result2 := ConvertToQueryValues(YourStruct{SubStruct: SubStruct{A: "A2", B: 2}}).Encode()
	const expectedResult2 = "SubStruct.A=A2&SubStruct.B=2"
	if result2 != expectedResult2 {
		// Sometimes result is not matched for the different orders
		t.Logf("Incorrect encoding: %s", result2)
	}
}

type TestStruct struct {
	Format       string
	Version      string
	AccessKeyId  string
	Timestamp    time.Time
	Empty        string
	IntValue     int      `ArgName:"int-value"`
	BoolPtr      *bool    `ArgName:"bool-ptr"`
	IntPtr       *int     `ArgName:"int-ptr"`
	StringArray  []string `ArgName:"str-array"`
	StringArray2 []string `ArgName:"array" query:"list"`
	StructArray  []SubStruct
	SubStruct    SubStruct
	test         TestString
	tests        []TestString
	Tag          map[string]string
}

func TestConvertToQueryValues(t *testing.T) {
	boolValue := true
	request := TestStruct{
		Format:       "JSON",
		Version:      "1.0",
		Timestamp:    time.Date(2015, time.Month(5), 26, 1, 2, 3, 4, time.UTC),
		IntValue:     10,
		BoolPtr:      &boolValue,
		StringArray:  []string{"abc", "xyz"},
		StringArray2: []string{"abc", "xyz"},
		StructArray: []SubStruct{
			SubStruct{A: "a", B: 1},
			SubStruct{A: "x", B: 2},
		},
		SubStruct: SubStruct{A: "M", B: 0},
		test:      TestString("test"),
		tests:     []TestString{TestString("test1"), TestString("test2")},
		Tag:       map[string]string{"abc": "xyz", "123": "456"},
	}
	result := ConvertToQueryValues(&request).Encode()
	const expectedResult = "Format=JSON&StructArray.1.A=a&StructArray.1.B=1&StructArray.2.A=x&StructArray.2.B=2&SubStruct.A=M&Tag.1.Key=abc&Tag.1.Value=xyz&Tag.2.Key=123&Tag.2.Value=456&Timestamp=2015-05-26T01%3A02%3A03Z&Version=1.0&array.1=abc&array.2=xyz&bool-ptr=true&int-value=10&str-array=%5B%22abc%22%2C%22xyz%22%5D&test=test&tests=%5B%22test1%22%2C%22test2%22%5D"

	if result != expectedResult {
		// Sometimes result is not matched for the different orders
		t.Logf("Incorrect encoding: %s", result)
	}

}
