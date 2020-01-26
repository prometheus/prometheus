package server

import (
	"reflect"
	"testing"

	"google.golang.org/genproto/protobuf/field_mask"
)

func TestApplyFieldMask(t *testing.T) {
	for _, test := range []struct {
		name      string
		patchee   interface{}
		patcher   interface{}
		fieldMask *field_mask.FieldMask
		expected  interface{}
	}{
		{"nil fieldMask", &a{E: 64}, &a{E: 42}, nil, &a{E: 64}},
		{"empty paths", &a{E: 63}, &a{E: 42}, &field_mask.FieldMask{}, &a{E: 63}},
		{"simple path", &a{E: 23, F: "test"}, &a{B: &b{}, E: 42}, &field_mask.FieldMask{Paths: []string{"E"}}, &a{E: 42, F: "test"}},
		{"nested", &a{B: &b{C: 85}}, &a{B: &b{C: 58, D: nil}}, &field_mask.FieldMask{Paths: []string{"B.C"}}, &a{B: &b{C: 58}}},
		{"multiple paths", &a{B: &b{C: 40, D: []int{1, 2, 3}}, E: 34, F: "catapult"}, &a{B: &b{C: 56}, F: "lettuce"}, &field_mask.FieldMask{Paths: []string{"B.C", "F"}}, &a{B: &b{C: 56, D: []int{1, 2, 3}}, E: 34, F: "lettuce"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			applyFieldMask(test.patchee, test.patcher, test.fieldMask)
			if !reflect.DeepEqual(test.patchee, test.expected) {
				t.Errorf("expected %v, but was %v", test.expected, test.patchee)
			}
		})
	}
}

func TestGetValue(t *testing.T) {
	for _, test := range []struct {
		name     string
		input    interface{}
		path     string
		expected interface{}
	}{
		{"empty", &a{E: 45, F: "test"}, "", &a{E: 45, F: "test"}},
		{"pointer-simple", &a{E: 45}, "E", 45},
		{"pointer-nested", &a{B: &b{C: 42}}, "B.C", 42},
		{"pointer-complex type", &a{B: &b{D: []int{1, 2}}}, "B.D", []int{1, 2}},
		{"pointer-invalid path", &a{F: "test"}, "X.Y", nil},
		{"simple", a{E: 45}, "E", 45},
		{"nested", a{B: &b{C: 42}}, "B.C", 42},
		{"complex type", a{B: &b{D: []int{1, 2}}}, "B.D", []int{1, 2}},
		{"invalid path", a{F: "test"}, "X.Y", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			if actual := getField(test.input, test.path); actual.IsValid() {
				if !reflect.DeepEqual(test.expected, actual.Interface()) {
					t.Errorf("expected %v, but got %v", test.expected, actual)
				}
			} else if test.expected != nil {
				t.Errorf("expected nil, but was %v", actual)
			}
		})
	}
}

func TestSetValue(t *testing.T) {
	for _, test := range []struct {
		name     string
		obj      interface{}
		newValue interface{}
		path     string
		expected interface{}
	}{
		{"simple", &a{E: 45}, 34, "E", 34},
		{"nested", &a{B: &b{C: 54}}, 43, "B.C", 43},
		{"complex type", &a{B: &b{D: []int{1, 2}}}, []int{3, 4}, "B.D", []int{3, 4}},
	} {
		t.Run(test.name, func(t *testing.T) {
			setValue(test.obj, reflect.ValueOf(test.newValue), test.path)
			if actual := getField(test.obj, test.path).Interface(); !reflect.DeepEqual(actual, test.expected) {
				t.Errorf("expected %v, but got %v", test.newValue, actual)
			}
		})
	}
}

type a struct {
	B *b
	E int
	F string
}

type b struct {
	C int
	D []int
}
