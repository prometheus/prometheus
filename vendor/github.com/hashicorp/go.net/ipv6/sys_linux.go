// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import (
	"net"
	"syscall"
)

// RFC 3542 options
const (
	// See /usr/include/linux/ipv6.h,in6.h.
	sysSockoptReceiveTrafficClass = syscall.IPV6_RECVTCLASS
	sysSockoptTrafficClass        = syscall.IPV6_TCLASS
	sysSockoptReceiveHopLimit     = syscall.IPV6_RECVHOPLIMIT
	sysSockoptHopLimit            = syscall.IPV6_HOPLIMIT
	sysSockoptReceivePacketInfo   = syscall.IPV6_RECVPKTINFO
	sysSockoptPacketInfo          = syscall.IPV6_PKTINFO
	sysSockoptReceivePathMTU      = 0x3c // IPV6_RECVPATHMTU
	sysSockoptPathMTU             = 0x3d // IPV6_PATHMTU
	sysSockoptNextHop             = syscall.IPV6_NEXTHOP
	sysSockoptChecksum            = syscall.IPV6_CHECKSUM

	// See /usr/include/linux/icmpv6.h.
	sysSockoptICMPFilter = 0x1 // syscall.ICMPV6_FILTER
)

func setSockaddr(sa *syscall.RawSockaddrInet6, ip net.IP, ifindex int) {
	sa.Family = syscall.AF_INET6
	copy(sa.Addr[:], ip)
	sa.Scope_id = uint32(ifindex)
}
