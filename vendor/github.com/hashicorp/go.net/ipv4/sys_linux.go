// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv4

type sysSockoptLen int32

var (
	ctlOpts = [ctlMax]ctlOpt{
		ctlTTL:        {sysIP_TTL, 1, marshalTTL, parseTTL},
		ctlPacketInfo: {sysIP_PKTINFO, sysSizeofInetPktinfo, marshalPacketInfo, parsePacketInfo},
	}

	sockOpts = [ssoMax]sockOpt{
		ssoTOS:                {sysIP_TOS, ssoTypeInt},
		ssoTTL:                {sysIP_TTL, ssoTypeInt},
		ssoMulticastTTL:       {sysIP_MULTICAST_TTL, ssoTypeInt},
		ssoMulticastInterface: {sysIP_MULTICAST_IF, ssoTypeIPMreqn},
		ssoMulticastLoopback:  {sysIP_MULTICAST_LOOP, ssoTypeInt},
		ssoReceiveTTL:         {sysIP_RECVTTL, ssoTypeInt},
		ssoPacketInfo:         {sysIP_PKTINFO, ssoTypeInt},
		ssoHeaderPrepend:      {sysIP_HDRINCL, ssoTypeInt},
		ssoJoinGroup:          {sysIP_ADD_MEMBERSHIP, ssoTypeIPMreqn},
		ssoLeaveGroup:         {sysIP_DROP_MEMBERSHIP, ssoTypeIPMreqn},
	}
)

func (pi *sysInetPktinfo) setIfindex(i int) {
	pi.Ifindex = int32(i)
}
