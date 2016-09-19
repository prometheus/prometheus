// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd linux netbsd openbsd

package ipv6

import (
	"net"
	"os"
	"syscall"
	"unsafe"
)

func ipv6TrafficClass(fd int) (int, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, sysSockoptTrafficClass)
	if err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	return v, nil
}

func setIPv6TrafficClass(fd, v int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, sysSockoptTrafficClass, v))
}

func ipv6HopLimit(fd int) (int, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, sysSockoptUnicastHopLimit)
	if err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	return v, nil
}

func setIPv6HopLimit(fd, v int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, sysSockoptUnicastHopLimit, v))
}

func ipv6Checksum(fd int) (bool, int, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, sysSockoptChecksum)
	if err != nil {
		return false, 0, os.NewSyscallError("getsockopt", err)
	}
	on := true
	if v == -1 {
		on = false
	}
	return on, v, nil
}

func ipv6MulticastHopLimit(fd int) (int, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, sysSockoptMulticastHopLimit)
	if err != nil {
		return 0, os.NewSyscallError("getsockopt", err)
	}
	return v, nil
}

func setIPv6MulticastHopLimit(fd, v int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, sysSockoptMulticastHopLimit, v))
}

func ipv6MulticastInterface(fd int) (*net.Interface, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, sysSockoptMulticastInterface)
	if err != nil {
		return nil, os.NewSyscallError("getsockopt", err)
	}
	if v == 0 {
		return nil, nil
	}
	ifi, err := net.InterfaceByIndex(v)
	if err != nil {
		return nil, err
	}
	return ifi, nil
}

func setIPv6MulticastInterface(fd int, ifi *net.Interface) error {
	var v int
	if ifi != nil {
		v = ifi.Index
	}
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, sysSockoptMulticastInterface, v))
}

func ipv6MulticastLoopback(fd int) (bool, error) {
	v, err := syscall.GetsockoptInt(fd, ianaProtocolIPv6, sysSockoptMulticastLoopback)
	if err != nil {
		return false, os.NewSyscallError("getsockopt", err)
	}
	return v == 1, nil
}

func setIPv6MulticastLoopback(fd int, v bool) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolIPv6, sysSockoptMulticastLoopback, boolint(v)))
}

func joinIPv6Group(fd int, ifi *net.Interface, grp net.IP) error {
	mreq := sysMulticastReq{}
	copy(mreq.IP[:], grp)
	if ifi != nil {
		mreq.IfIndex = uint32(ifi.Index)
	}
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6, sysSockoptJoinGroup, unsafe.Pointer(&mreq), sysSizeofMulticastReq))
}

func leaveIPv6Group(fd int, ifi *net.Interface, grp net.IP) error {
	mreq := sysMulticastReq{}
	copy(mreq.IP[:], grp)
	if ifi != nil {
		mreq.IfIndex = uint32(ifi.Index)
	}
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6, sysSockoptLeaveGroup, unsafe.Pointer(&mreq), sysSizeofMulticastReq))
}
