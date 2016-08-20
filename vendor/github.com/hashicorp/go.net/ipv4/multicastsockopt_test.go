// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv4_test

import (
	"net"
	"os"
	"runtime"
	"testing"

	"code.google.com/p/go.net/internal/nettest"
	"code.google.com/p/go.net/ipv4"
)

var packetConnMulticastSocketOptionTests = []struct {
	net, proto, addr string
	gaddr            net.Addr
}{
	{"udp4", "", "224.0.0.0:0", &net.UDPAddr{IP: net.IPv4(224, 0, 0, 249)}}, // see RFC 4727
	{"ip4", ":icmp", "0.0.0.0", &net.IPAddr{IP: net.IPv4(224, 0, 0, 250)}},  // see RFC 4727
}

func TestPacketConnMulticastSocketOptions(t *testing.T) {
	switch runtime.GOOS {
	case "nacl", "plan9", "solaris":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	ifi := nettest.RoutedInterface("ip4", net.FlagUp|net.FlagMulticast|net.FlagLoopback)
	if ifi == nil {
		t.Skipf("not available on %q", runtime.GOOS)
	}

	for _, tt := range packetConnMulticastSocketOptionTests {
		if tt.net == "ip4" && os.Getuid() != 0 {
			t.Skip("must be root")
		}
		c, err := net.ListenPacket(tt.net+tt.proto, tt.addr)
		if err != nil {
			t.Fatalf("net.ListenPacket failed: %v", err)
		}
		defer c.Close()

		testMulticastSocketOptions(t, ipv4.NewPacketConn(c), ifi, tt.gaddr)
	}
}

func TestRawConnMulticastSocketOptions(t *testing.T) {
	switch runtime.GOOS {
	case "nacl", "plan9", "solaris":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if os.Getuid() != 0 {
		t.Skip("must be root")
	}
	ifi := nettest.RoutedInterface("ip4", net.FlagUp|net.FlagMulticast|net.FlagLoopback)
	if ifi == nil {
		t.Skipf("not available on %q", runtime.GOOS)
	}

	c, err := net.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		t.Fatalf("net.ListenPacket failed: %v", err)
	}
	defer c.Close()

	r, err := ipv4.NewRawConn(c)
	if err != nil {
		t.Fatalf("ipv4.NewRawConn failed: %v", err)
	}

	testMulticastSocketOptions(t, r, ifi, &net.IPAddr{IP: net.IPv4(224, 0, 0, 250)}) /// see RFC 4727
}

type testIPv4MulticastConn interface {
	MulticastTTL() (int, error)
	SetMulticastTTL(ttl int) error
	MulticastLoopback() (bool, error)
	SetMulticastLoopback(bool) error
	JoinGroup(*net.Interface, net.Addr) error
	LeaveGroup(*net.Interface, net.Addr) error
}

func testMulticastSocketOptions(t *testing.T, c testIPv4MulticastConn, ifi *net.Interface, gaddr net.Addr) {
	const ttl = 255
	if err := c.SetMulticastTTL(ttl); err != nil {
		t.Fatalf("ipv4.PacketConn.SetMulticastTTL failed: %v", err)
	}
	if v, err := c.MulticastTTL(); err != nil {
		t.Fatalf("ipv4.PacketConn.MulticastTTL failed: %v", err)
	} else if v != ttl {
		t.Fatalf("got unexpected multicast TTL value %v; expected %v", v, ttl)
	}

	for _, toggle := range []bool{true, false} {
		if err := c.SetMulticastLoopback(toggle); err != nil {
			t.Fatalf("ipv4.PacketConn.SetMulticastLoopback failed: %v", err)
		}
		if v, err := c.MulticastLoopback(); err != nil {
			t.Fatalf("ipv4.PacketConn.MulticastLoopback failed: %v", err)
		} else if v != toggle {
			t.Fatalf("got unexpected multicast loopback %v; expected %v", v, toggle)
		}
	}

	if err := c.JoinGroup(ifi, gaddr); err != nil {
		t.Fatalf("ipv4.PacketConn.JoinGroup(%v, %v) failed: %v", ifi, gaddr, err)
	}
	if err := c.LeaveGroup(ifi, gaddr); err != nil {
		t.Fatalf("ipv4.PacketConn.LeaveGroup(%v, %v) failed: %v", ifi, gaddr, err)
	}
}
