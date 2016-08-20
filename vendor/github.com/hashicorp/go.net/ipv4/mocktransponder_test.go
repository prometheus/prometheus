// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv4_test

import (
	"net"
	"testing"
)

func acceptor(t *testing.T, ln net.Listener, done chan<- bool) {
	defer func() { done <- true }()

	c, err := ln.Accept()
	if err != nil {
		t.Errorf("net.Listener.Accept failed: %v", err)
		return
	}
	c.Close()
}
