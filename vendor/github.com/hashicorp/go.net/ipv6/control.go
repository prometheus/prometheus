// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import (
	"errors"
	"fmt"
	"net"
	"sync"
)

var (
	errMissingAddress  = errors.New("missing address")
	errInvalidConnType = errors.New("invalid conn type")
	errNoSuchInterface = errors.New("no such interface")
)

// References:
//
// RFC 2292  Advanced Sockets API for IPv6
//	http://tools.ietf.org/html/rfc2292
// RFC 2460  Internet Protocol, Version 6 (IPv6) Specification
//	http://tools.ietf.org/html/rfc2460
// RFC 3493  Basic Socket Interface Extensions for IPv6
//	http://tools.ietf.org/html/rfc3493.html
// RFC 3542  Advanced Sockets Application Program Interface (API) for IPv6
//	http://tools.ietf.org/html/rfc3542
//
// Note that RFC 3542 obsoletes RFC 2292 but OS X Snow Leopard and the
// former still support RFC 2292 only.  Please be aware that almost
// all protocol implementations prohibit using a combination of RFC
// 2292 and RFC 3542 for some practical reasons.

type rawOpt struct {
	sync.Mutex
	cflags ControlFlags
}

func (c *rawOpt) set(f ControlFlags)        { c.cflags |= f }
func (c *rawOpt) clear(f ControlFlags)      { c.cflags &^= f }
func (c *rawOpt) isset(f ControlFlags) bool { return c.cflags&f != 0 }

// A ControlFlags represents per packet basis IP-level socket option
// control flags.
type ControlFlags uint

const (
	FlagTrafficClass ControlFlags = 1 << iota // pass the traffic class on the received packet
	FlagHopLimit                              // pass the hop limit on the received packet
	FlagSrc                                   // pass the source address on the received packet
	FlagDst                                   // pass the destination address on the received packet
	FlagInterface                             // pass the interface index on the received packet
	FlagPathMTU                               // pass the path MTU on the received packet path
)

// A ControlMessage represents per packet basis IP-level socket
// options.
type ControlMessage struct {
	// Receiving socket options: SetControlMessage allows to
	// receive the options from the protocol stack using ReadFrom
	// method of PacketConn.
	//
	// Specifying socket options: ControlMessage for WriteTo
	// method of PacketConn allows to send the options to the
	// protocol stack.
	//
	TrafficClass int    // traffic class, must be 1 <= value <= 255 when specifying
	HopLimit     int    // hop limit, must be 1 <= value <= 255 when specifying
	Src          net.IP // source address, specifying only
	Dst          net.IP // destination address, receiving only
	IfIndex      int    // interface index, must be 1 <= value when specifying
	NextHop      net.IP // next hop address, specifying only
	MTU          int    // path MTU, receiving only
}

func (cm *ControlMessage) String() string {
	if cm == nil {
		return "<nil>"
	}
	return fmt.Sprintf("tclass: %#x, hoplim: %v, src: %v, dst: %v, ifindex: %v, nexthop: %v, mtu: %v", cm.TrafficClass, cm.HopLimit, cm.Src, cm.Dst, cm.IfIndex, cm.NextHop, cm.MTU)
}
