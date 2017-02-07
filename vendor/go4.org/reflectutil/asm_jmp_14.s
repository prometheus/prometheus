// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !go1.5,!js,!safe,!appengine
// +build amd64 386

#include "textflag.h"
#include "funcdata.h"

// func memmove(dst, src unsafe.Pointer, size uintptr)
TEXT ·memmove(SB),(NOSPLIT|WRAPPER),$0-24
	JMP	runtime·memmove(SB)
