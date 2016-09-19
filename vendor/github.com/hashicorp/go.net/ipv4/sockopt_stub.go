// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build nacl plan9 solaris

package ipv4

import "net"

func getInt(fd int, opt *sockOpt) (int, error) {
	return 0, errOpNoSupport
}

func setInt(fd int, opt *sockOpt, v int) error {
	return errOpNoSupport
}

func getInterface(fd int, opt *sockOpt) (*net.Interface, error) {
	return nil, errOpNoSupport
}

func setInterface(fd int, opt *sockOpt, ifi *net.Interface) error {
	return errOpNoSupport
}

func setGroup(fd int, opt *sockOpt, ifi *net.Interface, ip net.IP) error {
	return errOpNoSupport
}
