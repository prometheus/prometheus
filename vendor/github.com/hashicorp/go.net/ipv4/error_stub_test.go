// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build nacl plan9

package ipv4_test

func protocolNotSupported(err error) bool {
	return false
}
