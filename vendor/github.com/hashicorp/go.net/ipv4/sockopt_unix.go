// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd linux netbsd openbsd

package ipv4

import (
	"net"
	"os"
	"unsafe"

	"github.com/hashicorp/go.net/internal/iana"
)

func getInt(fd int, opt *sockOpt) (int, error) {
	if opt.name < 1 || (opt.typ != ssoTypeByte && opt.typ != ssoTypeInt) {
		return 0, errOpNoSupport
	}
	var i int32
	var b byte
	p := unsafe.Pointer(&i)
	l := sysSockoptLen(4)
	if opt.typ == ssoTypeByte {
		p = unsafe.Pointer(&b)
		l = sysSockoptLen(1)
	}
	if err := getsockopt(fd, iana.ProtocolIP, opt.name, p, &l); err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	if opt.typ == ssoTypeByte {
		return int(b), nil
	}
	return int(i), nil
}

func setInt(fd int, opt *sockOpt, v int) error {
	if opt.name < 1 || (opt.typ != ssoTypeByte && opt.typ != ssoTypeInt) {
		return errOpNoSupport
	}
	i := int32(v)
	var b byte
	p := unsafe.Pointer(&i)
	l := sysSockoptLen(4)
	if opt.typ == ssoTypeByte {
		b = byte(v)
		p = unsafe.Pointer(&b)
		l = sysSockoptLen(1)
	}
	return os.NewSyscallError("setsockopt", setsockopt(fd, iana.ProtocolIP, opt.name, p, l))
}

func getInterface(fd int, opt *sockOpt) (*net.Interface, error) {
	if opt.name < 1 {
		return nil, errOpNoSupport
	}
	switch opt.typ {
	case ssoTypeInterface:
		return getsockoptInterface(fd, opt.name)
	case ssoTypeIPMreqn:
		return getsockoptIPMreqn(fd, opt.name)
	default:
		return nil, errOpNoSupport
	}
}

func setInterface(fd int, opt *sockOpt, ifi *net.Interface) error {
	if opt.name < 1 {
		return errOpNoSupport
	}
	switch opt.typ {
	case ssoTypeInterface:
		return setsockoptInterface(fd, opt.name, ifi)
	case ssoTypeIPMreqn:
		return setsockoptIPMreqn(fd, opt.name, ifi, nil)
	default:
		return errOpNoSupport
	}
}

func setGroup(fd int, opt *sockOpt, ifi *net.Interface, grp net.IP) error {
	if opt.name < 1 {
		return errOpNoSupport
	}
	switch opt.typ {
	case ssoTypeIPMreq:
		return setsockoptIPMreq(fd, opt.name, ifi, grp)
	case ssoTypeIPMreqn:
		return setsockoptIPMreqn(fd, opt.name, ifi, grp)
	default:
		return errOpNoSupport
	}
}
