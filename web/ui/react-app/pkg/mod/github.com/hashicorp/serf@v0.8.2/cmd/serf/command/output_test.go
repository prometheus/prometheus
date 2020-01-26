package command

import (
	"fmt"
	"testing"
)

type OutputTest struct {
	XMLName    string           `json:"-"`
	TestString string           `json:"test_string"`
	TestInt    int              `json:"test_int"`
	TestNil    []byte           `json:"test_nil"`
	TestNested OutputTestNested `json:"nested"`
}

type OutputTestNested struct {
	NestKey string `json:"nest_key"`
}

func (o OutputTest) String() string {
	return fmt.Sprintf("%s    %d    %s", o.TestString, o.TestInt, o.TestNil)
}

func TestCommandOutput(t *testing.T) {
	var formatted []byte
	result := OutputTest{
		TestString: "woooo a string",
		TestInt:    77,
		TestNil:    nil,
		TestNested: OutputTestNested{
			NestKey: "nest_value",
		},
	}

	json_expected := `{
  "test_string": "woooo a string",
  "test_int": 77,
  "test_nil": null,
  "nested": {
    "nest_key": "nest_value"
  }
}`
	formatted, _ = formatOutput(result, "json")
	if string(formatted) != json_expected {
		t.Fatalf("bad json:\n%s\n\nexpected:\n%s", formatted, json_expected)
	}

	text_expected := "woooo a string    77"
	formatted, _ = formatOutput(result, "text")
	if string(formatted) != text_expected {
		t.Fatalf("bad output:\n\"%s\"\n\nexpected:\n\"%s\"", formatted, text_expected)
	}

	error_expected := `Invalid output format "boo"`
	_, err := formatOutput(result, "boo")
	if err.Error() != error_expected {
		t.Fatalf("bad output:\n\"%s\"\n\nexpected:\n\"%s\"", err.Error(), error_expected)
	}
}
