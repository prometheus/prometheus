// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package reflectutil contains reflect utilities.
package reflectutil

import "reflect"

// hasPointers reports whether the given type contains any pointers,
// including any internal pointers in slices, funcs, maps, channels,
// etc.
//
// This function exists for Swapper's internal use, instead of reaching
// into the runtime's *reflect._rtype kind&kindNoPointers flag.
func hasPointers(t reflect.Type) bool {
	if t == nil {
		panic("nil Type")
	}
	k := t.Kind()
	if k <= reflect.Complex128 {
		return false
	}
	switch k {
	default:
		// chan, func, interface, map, ptr, slice, string, unsafepointer
		// And anything else. It's safer to err on the side of true.
		return true
	case reflect.Array:
		return hasPointers(t.Elem())
	case reflect.Struct:
		num := t.NumField()
		for i := 0; i < num; i++ {
			if hasPointers(t.Field(i).Type) {
				return true
			}
		}
		return false
	}
}
