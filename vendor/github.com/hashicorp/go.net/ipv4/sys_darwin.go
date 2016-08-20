// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv4

import (
	"net"
	"syscall"
)

type sysSockoptLen int32

var (
	ctlOpts = [ctlMax]ctlOpt{
		ctlTTL:       {sysIP_RECVTTL, 1, marshalTTL, parseTTL},
		ctlDst:       {sysIP_RECVDSTADDR, net.IPv4len, marshalDst, parseDst},
		ctlInterface: {sysIP_RECVIF, syscall.SizeofSockaddrDatalink, marshalInterface, parseInterface},
	}

	sockOpts = [ssoMax]sockOpt{
		ssoTOS:                {sysIP_TOS, ssoTypeInt},
		ssoTTL:                {sysIP_TTL, ssoTypeInt},
		ssoMulticastTTL:       {sysIP_MULTICAST_TTL, ssoTypeByte},
		ssoMulticastInterface: {sysIP_MULTICAST_IF, ssoTypeInterface},
		ssoMulticastLoopback:  {sysIP_MULTICAST_LOOP, ssoTypeInt},
		ssoReceiveTTL:         {sysIP_RECVTTL, ssoTypeInt},
		ssoReceiveDst:         {sysIP_RECVDSTADDR, ssoTypeInt},
		ssoReceiveInterface:   {sysIP_RECVIF, ssoTypeInt},
		ssoHeaderPrepend:      {sysIP_HDRINCL, ssoTypeInt},
		ssoJoinGroup:          {sysIP_ADD_MEMBERSHIP, ssoTypeIPMreq},
		ssoLeaveGroup:         {sysIP_DROP_MEMBERSHIP, ssoTypeIPMreq},
	}
)

func init() {
	// Seems like kern.osreldate is veiled on latest OS X. We use
	// kern.osrelease instead.
	osver, err := syscall.Sysctl("kern.osrelease")
	if err != nil {
		return
	}
	var i int
	for i = range osver {
		if osver[i] != '.' {
			continue
		}
	}
	// The IP_PKTINFO was introduced in OS X 10.7 (Darwin
	// 11.0.0). See http://support.apple.com/kb/HT1633.
	if i > 2 || i == 2 && osver[0] >= '1' && osver[1] >= '1' {
		ctlOpts[ctlPacketInfo].name = sysIP_PKTINFO
		ctlOpts[ctlPacketInfo].length = sysSizeofInetPktinfo
		ctlOpts[ctlPacketInfo].marshal = marshalPacketInfo
		ctlOpts[ctlPacketInfo].parse = parsePacketInfo
		sockOpts[ssoPacketInfo].name = sysIP_RECVPKTINFO
		sockOpts[ssoPacketInfo].typ = ssoTypeInt
		sockOpts[ssoMulticastInterface].typ = ssoTypeIPMreqn
	}
}

func (pi *sysInetPktinfo) setIfindex(i int) {
	pi.Ifindex = uint32(i)
}
