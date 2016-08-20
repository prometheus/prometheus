// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

// +godefs map struct_in_addr [4]byte /* in_addr */

package ipv4

/*
#include <netinet/in.h>
*/
import "C"

const (
	sysIP_OPTIONS     = C.IP_OPTIONS
	sysIP_HDRINCL     = C.IP_HDRINCL
	sysIP_TOS         = C.IP_TOS
	sysIP_TTL         = C.IP_TTL
	sysIP_RECVOPTS    = C.IP_RECVOPTS
	sysIP_RECVRETOPTS = C.IP_RECVRETOPTS
	sysIP_RECVDSTADDR = C.IP_RECVDSTADDR
	sysIP_SENDSRCADDR = C.IP_SENDSRCADDR
	sysIP_RETOPTS     = C.IP_RETOPTS
	sysIP_RECVIF      = C.IP_RECVIF
	sysIP_ONESBCAST   = C.IP_ONESBCAST
	sysIP_BINDANY     = C.IP_BINDANY
	sysIP_RECVTTL     = C.IP_RECVTTL
	sysIP_MINTTL      = C.IP_MINTTL
	sysIP_DONTFRAG    = C.IP_DONTFRAG
	sysIP_RECVTOS     = C.IP_RECVTOS

	sysIP_MULTICAST_IF           = C.IP_MULTICAST_IF
	sysIP_MULTICAST_TTL          = C.IP_MULTICAST_TTL
	sysIP_MULTICAST_LOOP         = C.IP_MULTICAST_LOOP
	sysIP_ADD_MEMBERSHIP         = C.IP_ADD_MEMBERSHIP
	sysIP_DROP_MEMBERSHIP        = C.IP_DROP_MEMBERSHIP
	sysIP_MULTICAST_VIF          = C.IP_MULTICAST_VIF
	sysIP_ADD_SOURCE_MEMBERSHIP  = C.IP_ADD_SOURCE_MEMBERSHIP
	sysIP_DROP_SOURCE_MEMBERSHIP = C.IP_DROP_SOURCE_MEMBERSHIP
	sysIP_BLOCK_SOURCE           = C.IP_BLOCK_SOURCE
	sysIP_UNBLOCK_SOURCE         = C.IP_UNBLOCK_SOURCE

	sysSizeofIPMreq       = C.sizeof_struct_ip_mreq
	sysSizeofIPMreqn      = C.sizeof_struct_ip_mreqn
	sysSizeofIPMreqSource = C.sizeof_struct_ip_mreq_source
)

type sysIPMreq C.struct_ip_mreq

type sysIPMreqn C.struct_ip_mreqn

type sysIPMreqSource C.struct_ip_mreq_source
