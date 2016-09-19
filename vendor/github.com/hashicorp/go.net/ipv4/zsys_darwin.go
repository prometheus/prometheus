// Created by cgo -godefs - DO NOT EDIT
// cgo -godefs defs_darwin.go

package ipv4

const (
	sysIP_OPTIONS     = 0x1
	sysIP_HDRINCL     = 0x2
	sysIP_TOS         = 0x3
	sysIP_TTL         = 0x4
	sysIP_RECVOPTS    = 0x5
	sysIP_RECVRETOPTS = 0x6
	sysIP_RECVDSTADDR = 0x7
	sysIP_RETOPTS     = 0x8
	sysIP_RECVIF      = 0x14
	sysIP_STRIPHDR    = 0x17
	sysIP_RECVTTL     = 0x18
	sysIP_BOUND_IF    = 0x19
	sysIP_PKTINFO     = 0x1a
	sysIP_RECVPKTINFO = 0x1a

	sysIP_MULTICAST_IF           = 0x9
	sysIP_MULTICAST_TTL          = 0xa
	sysIP_MULTICAST_LOOP         = 0xb
	sysIP_ADD_MEMBERSHIP         = 0xc
	sysIP_DROP_MEMBERSHIP        = 0xd
	sysIP_MULTICAST_VIF          = 0xe
	sysIP_MULTICAST_IFINDEX      = 0x42
	sysIP_ADD_SOURCE_MEMBERSHIP  = 0x46
	sysIP_DROP_SOURCE_MEMBERSHIP = 0x47
	sysIP_BLOCK_SOURCE           = 0x48
	sysIP_UNBLOCK_SOURCE         = 0x49

	sysSizeofInetPktinfo = 0xc

	sysSizeofIPMreq       = 0x8
	sysSizeofIPMreqn      = 0xc
	sysSizeofIPMreqSource = 0xc
)

type sysInetPktinfo struct {
	Ifindex  uint32
	Spec_dst [4]byte /* in_addr */
	Addr     [4]byte /* in_addr */
}

type sysIPMreq struct {
	Multiaddr [4]byte /* in_addr */
	Interface [4]byte /* in_addr */
}

type sysIPMreqn struct {
	Multiaddr [4]byte /* in_addr */
	Address   [4]byte /* in_addr */
	Ifindex   int32
}

type sysIPMreqSource struct {
	Multiaddr  [4]byte /* in_addr */
	Sourceaddr [4]byte /* in_addr */
	Interface  [4]byte /* in_addr */
}
