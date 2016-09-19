// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd linux netbsd openbsd

package ipv6

import "syscall"

// RFC 3493 options
const (
	sysSockoptUnicastHopLimit    = syscall.IPV6_UNICAST_HOPS
	sysSockoptMulticastHopLimit  = syscall.IPV6_MULTICAST_HOPS
	sysSockoptMulticastInterface = syscall.IPV6_MULTICAST_IF
	sysSockoptMulticastLoopback  = syscall.IPV6_MULTICAST_LOOP
	sysSockoptJoinGroup          = syscall.IPV6_JOIN_GROUP
	sysSockoptLeaveGroup         = syscall.IPV6_LEAVE_GROUP
)

const sysSizeofMTUInfo = 0x20

type sysMTUInfo struct {
	Addr syscall.RawSockaddrInet6
	MTU  uint32
}
