// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build dragonfly freebsd linux netbsd openbsd

package ipv6

import (
	"os"
	"syscall"
	"unsafe"
)

func ipv6ReceiveTrafficClass(fd int) (bool, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, sysSockoptReceiveTrafficClass)
	if err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6ReceiveTrafficClass(fd int, v bool) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, sysSockoptReceiveTrafficClass, boolint(v)))
}

func ipv6ReceiveHopLimit(fd int) (bool, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, sysSockoptReceiveHopLimit)
	if err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6ReceiveHopLimit(fd int, v bool) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, sysSockoptReceiveHopLimit, boolint(v)))
}

func ipv6ReceivePacketInfo(fd int) (bool, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, sysSockoptReceivePacketInfo)
	if err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6ReceivePacketInfo(fd int, v bool) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, sysSockoptReceivePacketInfo, boolint(v)))
}

func ipv6PathMTU(fd int) (int, error) {
	var v sysMTUInfo
	l := sysSockoptLen(sysSizeofMTUInfo)
	if err := getsockopt(fd, ianaProtocolIPv6, sysSockoptPathMTU, unsafe.Pointer(&v), &l); err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	return int(v.MTU), nil
}

func ipv6ReceivePathMTU(fd int) (bool, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, sysSockoptReceivePathMTU)
	if err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6ReceivePathMTU(fd int, v bool) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, sysSockoptReceivePathMTU, boolint(v)))
}

func ipv6ICMPFilter(fd int) (*ICMPFilter, error) {
	var v ICMPFilter
	l := sysSockoptLen(sysSizeofICMPFilter)
	if err := getsockopt(fd, ianaProtocolIPv6ICMP, sysSockoptICMPFilter, unsafe.Pointer(&v.sysICMPFilter), &l); err != nil {
		return nil, os.NewSyscallError("getsockopt", err)
	}
	return &v, nil
}

func setIPv6ICMPFilter(fd int, f *ICMPFilter) error {
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6ICMP, sysSockoptICMPFilter, unsafe.Pointer(&f.sysICMPFilter), sysSizeofICMPFilter))
}
