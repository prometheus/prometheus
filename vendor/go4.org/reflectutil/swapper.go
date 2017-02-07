// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reflectutil

import "reflect"

// Swapper returns a function which swaps the elements in slice.
// Swapper panics if the provided interface is not a slice.
//
// Its goal is to work safely and efficiently for all versions and
// variants of Go: pre-Go1.5, Go1.5+, safe, unsafe, App Engine,
// GopherJS, etc.
func Swapper(slice interface{}) func(i, j int) {
	v := reflect.ValueOf(slice)
	if v.Kind() != reflect.Slice {
		panic(&reflect.ValueError{Method: "reflectutil.Swapper", Kind: v.Kind()})
	}
	return swapper(v)
}
