// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !js,!appengine,!safe

package reflectutil

import (
	"reflect"
	"unsafe"
)

const ptrSize = 4 << (^uintptr(0) >> 63) // unsafe.Sizeof(uintptr(0)) but an ideal const

// arrayAt returns the i-th element of p, a C-array whose elements are
// eltSize wide (in bytes).
func arrayAt(p unsafe.Pointer, i int, eltSize uintptr) unsafe.Pointer {
	return unsafe.Pointer(uintptr(p) + uintptr(i)*eltSize)
}

type sliceHeader struct {
	Data unsafe.Pointer
	Len  int
	Cap  int
}

func swapper(v reflect.Value) func(i, j int) {
	maxLen := uint(v.Len())

	s := sliceHeader{unsafe.Pointer(v.Pointer()), int(maxLen), int(maxLen)}
	tt := v.Type()
	elemt := tt.Elem()
	typ := unsafe.Pointer(reflect.ValueOf(elemt).Pointer())

	size := elemt.Size()
	hasPtr := hasPointers(elemt)

	// Some common & small cases, without using memmove:
	if hasPtr {
		if size == ptrSize {
			var ps []unsafe.Pointer
			*(*sliceHeader)(unsafe.Pointer(&ps)) = s
			return func(i, j int) { ps[i], ps[j] = ps[j], ps[i] }
		}
		if elemt.Kind() == reflect.String {
			var ss []string
			*(*sliceHeader)(unsafe.Pointer(&ss)) = s
			return func(i, j int) { ss[i], ss[j] = ss[j], ss[i] }
		}
	} else {
		switch size {
		case 8:
			var is []int64
			*(*sliceHeader)(unsafe.Pointer(&is)) = s
			return func(i, j int) { is[i], is[j] = is[j], is[i] }
		case 4:
			var is []int32
			*(*sliceHeader)(unsafe.Pointer(&is)) = s
			return func(i, j int) { is[i], is[j] = is[j], is[i] }
		case 2:
			var is []int16
			*(*sliceHeader)(unsafe.Pointer(&is)) = s
			return func(i, j int) { is[i], is[j] = is[j], is[i] }
		case 1:
			var is []int8
			*(*sliceHeader)(unsafe.Pointer(&is)) = s
			return func(i, j int) { is[i], is[j] = is[j], is[i] }
		}
	}

	// Allocate scratch space for swaps:
	tmpVal := reflect.New(elemt)
	tmp := unsafe.Pointer(tmpVal.Pointer())

	// If no pointers (or Go 1.4 or below), we don't require typedmemmove:
	if !haveTypedMemmove || !hasPtr {
		return func(i, j int) {
			if uint(i) >= maxLen || uint(j) >= maxLen {
				panic("reflect: slice index out of range")
			}
			val1 := arrayAt(s.Data, i, size)
			val2 := arrayAt(s.Data, j, size)
			memmove(tmp, val1, size)
			memmove(val1, val2, size)
			memmove(val2, tmp, size)
		}
	}

	return func(i, j int) {
		if uint(i) >= maxLen || uint(j) >= maxLen {
			panic("reflect: slice index out of range")
		}
		val1 := arrayAt(s.Data, i, size)
		val2 := arrayAt(s.Data, j, size)
		typedmemmove(typ, tmp, val1)
		typedmemmove(typ, val1, val2)
		typedmemmove(typ, val2, tmp)
	}
}

// memmove copies size bytes from src to dst.
// The memory must not contain any pointers.
//go:noescape
func memmove(dst, src unsafe.Pointer, size uintptr)
