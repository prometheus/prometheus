// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin

package fsnotify

import "syscall"

// note: this constant is not defined on BSD
const openMode = syscall.O_EVTONLY
