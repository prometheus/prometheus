// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build dragonfly freebsd netbsd openbsd

package ipv6

import (
	"net"
	"syscall"
)

// RFC 3542 options
const (
	// See /usr/include/netinet6/in6.h.
	sysSockoptReceiveTrafficClass = syscall.IPV6_RECVTCLASS
	sysSockoptTrafficClass        = syscall.IPV6_TCLASS
	sysSockoptReceiveHopLimit     = syscall.IPV6_RECVHOPLIMIT
	sysSockoptHopLimit            = syscall.IPV6_HOPLIMIT
	sysSockoptReceivePacketInfo   = syscall.IPV6_RECVPKTINFO
	sysSockoptPacketInfo          = syscall.IPV6_PKTINFO
	sysSockoptReceivePathMTU      = syscall.IPV6_RECVPATHMTU
	sysSockoptPathMTU             = syscall.IPV6_PATHMTU
	sysSockoptNextHop             = syscall.IPV6_NEXTHOP
	sysSockoptChecksum            = syscall.IPV6_CHECKSUM

	// See /usr/include/netinet6/in6.h.
	sysSockoptICMPFilter = 0x12 // syscall.ICMP6_FILTER
)

func setSockaddr(sa *syscall.RawSockaddrInet6, ip net.IP, ifindex int) {
	sa.Len = syscall.SizeofSockaddrInet6
	sa.Family = syscall.AF_INET6
	copy(sa.Addr[:], ip)
	sa.Scope_id = uint32(ifindex)
}
