// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !go1.5,!js,!appengine,!safe

package reflectutil

import "unsafe"

const haveTypedMemmove = false

func typedmemmove(reflect_rtype, dst, src unsafe.Pointer) {
	panic("never called") // only here so swapper_unsafe.go compiles
}
