// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package icmp provides basic functions for the manipulation of ICMP
// message.
package icmp

import (
	"errors"
	"net"

	"github.com/hashicorp/go.net/internal/iana"
	"github.com/hashicorp/go.net/ipv4"
	"github.com/hashicorp/go.net/ipv6"
)

// A Type represents an ICMP message type.
type Type interface {
	String() string
}

// A Message represents an ICMP message.
type Message struct {
	Type     Type        // type, either ipv4.ICMPType or ipv6.ICMPType
	Code     int         // code
	Checksum int         // checksum
	Body     MessageBody // body
}

// Marshal returns the binary enconding of the ICMP message m.
//
// For ICMP for IPv4 message, the returned message always contains the
// calculated checksum field.
//
// For ICMP for IPv6 message, the returned message contains the
// calculated checksum field when psh is not nil, otherwise the kernel
// will compute the checksum field during the message transmission.
// When psh is not nil, it must be the pseudo header for IPv6.
func (m *Message) Marshal(psh []byte) ([]byte, error) {
	var mtype int
	var icmpv6 bool
	switch typ := m.Type.(type) {
	case ipv4.ICMPType:
		mtype = int(typ)
	case ipv6.ICMPType:
		mtype = int(typ)
		icmpv6 = true
	default:
		return nil, errors.New("invalid argument")
	}
	b := []byte{byte(mtype), byte(m.Code), 0, 0}
	if icmpv6 && psh != nil {
		b = append(psh, b...)
	}
	if m.Body != nil && m.Body.Len() != 0 {
		mb, err := m.Body.Marshal()
		if err != nil {
			return nil, err
		}
		b = append(b, mb...)
	}
	if icmpv6 {
		if psh == nil { // cannot calculate checksum here
			return b, nil
		}
		off, l := 2*net.IPv6len, len(b)-len(psh)
		b[off], b[off+1], b[off+2], b[off+3] = byte(l>>24), byte(l>>16), byte(l>>8), byte(l)
	}
	csumcv := len(b) - 1 // checksum coverage
	s := uint32(0)
	for i := 0; i < csumcv; i += 2 {
		s += uint32(b[i+1])<<8 | uint32(b[i])
	}
	if csumcv&1 == 0 {
		s += uint32(b[csumcv])
	}
	s = s>>16 + s&0xffff
	s = s + s>>16
	// Place checksum back in header; using ^= avoids the
	// assumption the checksum bytes are zero.
	b[len(psh)+2] ^= byte(^s)
	b[len(psh)+3] ^= byte(^s >> 8)
	return b[len(psh):], nil
}

// ParseMessage parses b as an ICMP message. Proto must be
// iana.ProtocolICMP or iana.ProtocolIPv6ICMP.
func ParseMessage(proto int, b []byte) (*Message, error) {
	if len(b) < 4 {
		return nil, errors.New("message too short")
	}
	var err error
	switch proto {
	case iana.ProtocolICMP:
		m := &Message{Type: ipv4.ICMPType(b[0]), Code: int(b[1]), Checksum: int(b[2])<<8 | int(b[3])}
		switch m.Type {
		case ipv4.ICMPTypeEcho, ipv4.ICMPTypeEchoReply:
			m.Body, err = parseEcho(b[4:])
			if err != nil {
				return nil, err
			}
		default:
			m.Body = &DefaultMessageBody{Data: b[4:]}
		}
		return m, nil
	case iana.ProtocolIPv6ICMP:
		m := &Message{Type: ipv6.ICMPType(b[0]), Code: int(b[1]), Checksum: int(b[2])<<8 | int(b[3])}
		switch m.Type {
		case ipv6.ICMPTypeEchoRequest, ipv6.ICMPTypeEchoReply:
			m.Body, err = parseEcho(b[4:])
			if err != nil {
				return nil, err
			}
		default:
			m.Body = &DefaultMessageBody{Data: b[4:]}
		}
		return m, nil
	default:
		return nil, errors.New("unknown protocol")
	}
}
