/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package conversion

import (
	"fmt"
	"reflect"
)

// Cloner knows how to copy one type to another.
type Cloner struct {
	// Map from the type to a function which can do the deep copy.
	deepCopyFuncs          map[reflect.Type]reflect.Value
	generatedDeepCopyFuncs map[reflect.Type]func(in interface{}, out interface{}, c *Cloner) error
}

// NewCloner creates a new Cloner object.
func NewCloner() *Cloner {
	c := &Cloner{
		deepCopyFuncs:          map[reflect.Type]reflect.Value{},
		generatedDeepCopyFuncs: map[reflect.Type]func(in interface{}, out interface{}, c *Cloner) error{},
	}
	if err := c.RegisterDeepCopyFunc(byteSliceDeepCopy); err != nil {
		// If one of the deep-copy functions is malformed, detect it immediately.
		panic(err)
	}
	return c
}

// Prevent recursing into every byte...
func byteSliceDeepCopy(in *[]byte, out *[]byte, c *Cloner) error {
	if *in != nil {
		*out = make([]byte, len(*in))
		copy(*out, *in)
	} else {
		*out = nil
	}
	return nil
}

// Verifies whether a deep-copy function has a correct signature.
func verifyDeepCopyFunctionSignature(ft reflect.Type) error {
	if ft.Kind() != reflect.Func {
		return fmt.Errorf("expected func, got: %v", ft)
	}
	if ft.NumIn() != 3 {
		return fmt.Errorf("expected three 'in' params, got %v", ft)
	}
	if ft.NumOut() != 1 {
		return fmt.Errorf("expected one 'out' param, got %v", ft)
	}
	if ft.In(0).Kind() != reflect.Ptr {
		return fmt.Errorf("expected pointer arg for 'in' param 0, got: %v", ft)
	}
	if ft.In(1) != ft.In(0) {
		return fmt.Errorf("expected 'in' param 0 the same as param 1, got: %v", ft)
	}
	var forClonerType Cloner
	if expected := reflect.TypeOf(&forClonerType); ft.In(2) != expected {
		return fmt.Errorf("expected '%v' arg for 'in' param 2, got: '%v'", expected, ft.In(2))
	}
	var forErrorType error
	// This convolution is necessary, otherwise TypeOf picks up on the fact
	// that forErrorType is nil
	errorType := reflect.TypeOf(&forErrorType).Elem()
	if ft.Out(0) != errorType {
		return fmt.Errorf("expected error return, got: %v", ft)
	}
	return nil
}

// RegisterGeneratedDeepCopyFunc registers a copying func with the Cloner.
// deepCopyFunc must take three parameters: a type input, a pointer to a
// type output, and a pointer to Cloner. It should return an error.
//
// Example:
// c.RegisterGeneratedDeepCopyFunc(
//         func(in Pod, out *Pod, c *Cloner) error {
//                 // deep copy logic...
//                 return nil
//          })
func (c *Cloner) RegisterDeepCopyFunc(deepCopyFunc interface{}) error {
	fv := reflect.ValueOf(deepCopyFunc)
	ft := fv.Type()
	if err := verifyDeepCopyFunctionSignature(ft); err != nil {
		return err
	}
	c.deepCopyFuncs[ft.In(0)] = fv
	return nil
}

// GeneratedDeepCopyFunc bundles an untyped generated deep-copy function of a type
// with a reflection type object used as a key to lookup the deep-copy function.
type GeneratedDeepCopyFunc struct {
	Fn     func(in interface{}, out interface{}, c *Cloner) error
	InType reflect.Type
}

// Similar to RegisterDeepCopyFunc, but registers deep copy function that were
// automatically generated.
func (c *Cloner) RegisterGeneratedDeepCopyFunc(fn GeneratedDeepCopyFunc) error {
	c.generatedDeepCopyFuncs[fn.InType] = fn.Fn
	return nil
}

// DeepCopy will perform a deep copy of a given object.
func (c *Cloner) DeepCopy(in interface{}) (interface{}, error) {
	// Can be invalid if we run DeepCopy(X) where X is a nil interface type.
	// For example, we get an invalid value when someone tries to deep-copy
	// a nil labels.Selector.
	// This does not occur if X is nil and is a pointer to a concrete type.
	if in == nil {
		return nil, nil
	}
	inValue := reflect.ValueOf(in)
	outValue, err := c.deepCopy(inValue)
	if err != nil {
		return nil, err
	}
	return outValue.Interface(), nil
}

func (c *Cloner) deepCopy(src reflect.Value) (reflect.Value, error) {
	inType := src.Type()

	switch src.Kind() {
	case reflect.Interface, reflect.Ptr, reflect.Map, reflect.Slice:
		if src.IsNil() {
			return src, nil
		}
	}

	if fv, ok := c.deepCopyFuncs[inType]; ok {
		return c.customDeepCopy(src, fv)
	}
	if fv, ok := c.generatedDeepCopyFuncs[inType]; ok {
		var outValue reflect.Value
		outValue = reflect.New(inType.Elem())
		err := fv(src.Interface(), outValue.Interface(), c)
		return outValue, err
	}
	return c.defaultDeepCopy(src)
}

func (c *Cloner) customDeepCopy(src, fv reflect.Value) (reflect.Value, error) {
	outValue := reflect.New(src.Type().Elem())
	args := []reflect.Value{src, outValue, reflect.ValueOf(c)}
	result := fv.Call(args)[0].Interface()
	// This convolution is necessary because nil interfaces won't convert
	// to error.
	if result == nil {
		return outValue, nil
	}
	return outValue, result.(error)
}

func (c *Cloner) defaultDeepCopy(src reflect.Value) (reflect.Value, error) {
	switch src.Kind() {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer, reflect.Uintptr:
		return src, fmt.Errorf("cannot deep copy kind: %s", src.Kind())
	case reflect.Array:
		dst := reflect.New(src.Type())
		for i := 0; i < src.Len(); i++ {
			copyVal, err := c.deepCopy(src.Index(i))
			if err != nil {
				return src, err
			}
			dst.Elem().Index(i).Set(copyVal)
		}
		return dst.Elem(), nil
	case reflect.Interface:
		if src.IsNil() {
			return src, nil
		}
		return c.deepCopy(src.Elem())
	case reflect.Map:
		if src.IsNil() {
			return src, nil
		}
		dst := reflect.MakeMap(src.Type())
		for _, k := range src.MapKeys() {
			copyVal, err := c.deepCopy(src.MapIndex(k))
			if err != nil {
				return src, err
			}
			dst.SetMapIndex(k, copyVal)
		}
		return dst, nil
	case reflect.Ptr:
		if src.IsNil() {
			return src, nil
		}
		dst := reflect.New(src.Type().Elem())
		copyVal, err := c.deepCopy(src.Elem())
		if err != nil {
			return src, err
		}
		dst.Elem().Set(copyVal)
		return dst, nil
	case reflect.Slice:
		if src.IsNil() {
			return src, nil
		}
		dst := reflect.MakeSlice(src.Type(), 0, src.Len())
		for i := 0; i < src.Len(); i++ {
			copyVal, err := c.deepCopy(src.Index(i))
			if err != nil {
				return src, err
			}
			dst = reflect.Append(dst, copyVal)
		}
		return dst, nil
	case reflect.Struct:
		dst := reflect.New(src.Type())
		for i := 0; i < src.NumField(); i++ {
			if !dst.Elem().Field(i).CanSet() {
				// Can't set private fields. At this point, the
				// best we can do is a shallow copy. For
				// example, time.Time is a value type with
				// private members that can be shallow copied.
				return src, nil
			}
			copyVal, err := c.deepCopy(src.Field(i))
			if err != nil {
				return src, err
			}
			dst.Elem().Field(i).Set(copyVal)
		}
		return dst.Elem(), nil

	default:
		// Value types like numbers, booleans, and strings.
		return src, nil
	}
}
