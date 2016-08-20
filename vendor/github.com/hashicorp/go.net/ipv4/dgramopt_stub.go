// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build nacl plan9 solaris

package ipv4

import "net"

func (c *dgramOpt) MulticastTTL() (int, error) {
	// TODO(mikio): Implement this
	return 0, errOpNoSupport
}

func (c *dgramOpt) SetMulticastTTL(ttl int) error {
	// TODO(mikio): Implement this
	return errOpNoSupport
}

func (c *dgramOpt) MulticastInterface() (*net.Interface, error) {
	// TODO(mikio): Implement this
	return nil, errOpNoSupport
}

func (c *dgramOpt) SetMulticastInterface(ifi *net.Interface) error {
	// TODO(mikio): Implement this
	return errOpNoSupport
}

func (c *dgramOpt) MulticastLoopback() (bool, error) {
	// TODO(mikio): Implement this
	return false, errOpNoSupport
}

func (c *dgramOpt) SetMulticastLoopback(on bool) error {
	// TODO(mikio): Implement this
	return errOpNoSupport
}

func (c *dgramOpt) JoinGroup(ifi *net.Interface, grp net.Addr) error {
	// TODO(mikio): Implement this
	return errOpNoSupport
}

func (c *dgramOpt) LeaveGroup(ifi *net.Interface, grp net.Addr) error {
	// TODO(mikio): Implement this
	return errOpNoSupport
}
