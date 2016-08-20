// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package icmp_test

import (
	"net"
	"reflect"
	"testing"

	"github.com/hashicorp/go.net/internal/iana"
	"github.com/hashicorp/go.net/internal/icmp"
	"github.com/hashicorp/go.net/ipv4"
	"github.com/hashicorp/go.net/ipv6"
)

var marshalAndParseMessageForIPv4Tests = []icmp.Message{
	{
		Type: ipv4.ICMPTypeEcho, Code: 0,
		Body: &icmp.Echo{
			ID: 1, Seq: 2,
			Data: []byte("HELLO-R-U-THERE"),
		},
	},
	{
		Type: ipv4.ICMPTypePhoturis,
		Body: &icmp.DefaultMessageBody{
			Data: []byte{0x80, 0x40, 0x20, 0x10},
		},
	},
}

func TestMarshalAndParseMessageForIPv4(t *testing.T) {
	for _, tt := range marshalAndParseMessageForIPv4Tests {
		b, err := tt.Marshal(nil)
		if err != nil {
			t.Fatal(err)
		}
		m, err := icmp.ParseMessage(iana.ProtocolICMP, b)
		if err != nil {
			t.Fatal(err)
		}
		if m.Type != tt.Type || m.Code != tt.Code {
			t.Errorf("got %v; want %v", m, &tt)
		}
		if !reflect.DeepEqual(m.Body, tt.Body) {
			t.Errorf("got %v; want %v", m.Body, tt.Body)
		}
	}
}

var marshalAndParseMessageForIPv6Tests = []icmp.Message{
	{
		Type: ipv6.ICMPTypeEchoRequest, Code: 0,
		Body: &icmp.Echo{
			ID: 1, Seq: 2,
			Data: []byte("HELLO-R-U-THERE"),
		},
	},
	{
		Type: ipv6.ICMPTypeDuplicateAddressConfirmation,
		Body: &icmp.DefaultMessageBody{
			Data: []byte{0x80, 0x40, 0x20, 0x10},
		},
	},
}

func TestMarshalAndParseMessageForIPv6(t *testing.T) {
	pshicmp := icmp.IPv6PseudoHeader(net.ParseIP("fe80::1"), net.ParseIP("ff02::1"))
	for _, tt := range marshalAndParseMessageForIPv6Tests {
		for _, psh := range [][]byte{pshicmp, nil} {
			b, err := tt.Marshal(psh)
			if err != nil {
				t.Fatal(err)
			}
			m, err := icmp.ParseMessage(iana.ProtocolIPv6ICMP, b)
			if err != nil {
				t.Fatal(err)
			}
			if m.Type != tt.Type || m.Code != tt.Code {
				t.Errorf("got %v; want %v", m, &tt)
			}
			if !reflect.DeepEqual(m.Body, tt.Body) {
				t.Errorf("got %v; want %v", m.Body, tt.Body)
			}
		}
	}
}
