// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.6
// +build arm

#include "textflag.h"
#include "funcdata.h"

// func typedmemmove(reflect_rtype, src unsafe.Pointer, size uintptr)
TEXT ·typedmemmove(SB),(NOSPLIT|WRAPPER),$0-24
	B	runtime·typedmemmove(SB)

// func memmove(dst, src unsafe.Pointer, size uintptr)
TEXT ·memmove(SB),(NOSPLIT|WRAPPER),$0-24
	B	runtime·memmove(SB)
