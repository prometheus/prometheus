// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package windows

//go:generate go run $GOROOT/src/syscall/mksyscall_windows.go -output zsyscall_windows.go eventlog.go service.go syscall_windows.go security_windows.go
