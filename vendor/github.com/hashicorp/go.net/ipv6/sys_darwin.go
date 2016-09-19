// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import (
	"net"
	"syscall"
)

// RFC 2292 options
const (
	// See /usr/include/netinet6/in6.h.
	sysSockopt2292HopLimit   = syscall.IPV6_2292HOPLIMIT
	sysSockopt2292PacketInfo = syscall.IPV6_2292PKTINFO
	sysSockopt2292NextHop    = syscall.IPV6_2292NEXTHOP
)

// RFC 3542 options
const (
	// See /usr/include/netinet6/in6.h.
	sysSockoptReceiveTrafficClass = 0x23 // IPV6_RECVTCLASS
	sysSockoptTrafficClass        = 0x24 // IPV6_TCLASS
	sysSockoptReceiveHopLimit     = 0x25 // IPV6_RECVHOPLIMIT
	sysSockoptHopLimit            = 0x2f // IPV6_HOPLIMIT
	sysSockoptReceivePacketInfo   = 0x3d // IPV6_RECVPKTINFO
	sysSockoptPacketInfo          = 0x2e // IPV6_PKTINFO
	sysSockoptReceivePathMTU      = 0x2b // IPV6_RECVPATHMTU
	sysSockoptPathMTU             = 0x2c // IPV6_PATHMTU
	sysSockoptNextHop             = 0x30 // IPV6_NEXTHOP
	sysSockoptChecksum            = 0x1a // IPV6_CHECKSUM

	// See /usr/include/netinet6/in6.h.
	sysSockoptICMPFilter = 0x12 // ICMP6_FILTER
)

func setSockaddr(sa *syscall.RawSockaddrInet6, ip net.IP, ifindex int) {
	sa.Len = syscall.SizeofSockaddrInet6
	sa.Family = syscall.AF_INET6
	copy(sa.Addr[:], ip)
	sa.Scope_id = uint32(ifindex)
}
