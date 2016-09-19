// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import (
	"net"
	"syscall"
)

// RFC 3493 options
const (
	// See ws2tcpip.h.
	sysSockoptUnicastHopLimit    = syscall.IPV6_UNICAST_HOPS
	sysSockoptMulticastHopLimit  = syscall.IPV6_MULTICAST_HOPS
	sysSockoptMulticastInterface = syscall.IPV6_MULTICAST_IF
	sysSockoptMulticastLoopback  = syscall.IPV6_MULTICAST_LOOP
	sysSockoptJoinGroup          = syscall.IPV6_JOIN_GROUP
	sysSockoptLeaveGroup         = syscall.IPV6_LEAVE_GROUP
)

// RFC 3542 options
const (
	// See ws2tcpip.h.
	sysSockoptPacketInfo = 0x13 // IPV6_PKTINFO
)

func setSockaddr(sa *syscall.RawSockaddrInet6, ip net.IP, ifindex int) {
	sa.Family = syscall.AF_INET6
	copy(sa.Addr[:], ip)
	sa.Scope_id = uint32(ifindex)
}
