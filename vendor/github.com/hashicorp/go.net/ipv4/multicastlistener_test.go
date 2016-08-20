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

var udpMultipleGroupListenerTests = []net.Addr{
	&net.UDPAddr{IP: net.IPv4(224, 0, 0, 249)}, // see RFC 4727
	&net.UDPAddr{IP: net.IPv4(224, 0, 0, 250)},
	&net.UDPAddr{IP: net.IPv4(224, 0, 0, 254)},
}

func TestUDPSinglePacketConnWithMultipleGroupListeners(t *testing.T) {
	switch runtime.GOOS {
	case "nacl", "plan9", "solaris", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if testing.Short() {
		t.Skip("to avoid external network")
	}

	for _, gaddr := range udpMultipleGroupListenerTests {
		c, err := net.ListenPacket("udp4", "0.0.0.0:0") // wildcard address with no reusable port
		if err != nil {
			t.Fatalf("net.ListenPacket failed: %v", err)
		}
		defer c.Close()

		p := ipv4.NewPacketConn(c)
		var mift []*net.Interface

		ift, err := net.Interfaces()
		if err != nil {
			t.Fatalf("net.Interfaces failed: %v", err)
		}
		for i, ifi := range ift {
			if _, ok := nettest.IsMulticastCapable("ip4", &ifi); !ok {
				continue
			}
			if err := p.JoinGroup(&ifi, gaddr); err != nil {
				t.Fatalf("ipv4.PacketConn.JoinGroup %v on %v failed: %v", gaddr, ifi, err)
			}
			mift = append(mift, &ift[i])
		}
		for _, ifi := range mift {
			if err := p.LeaveGroup(ifi, gaddr); err != nil {
				t.Fatalf("ipv4.PacketConn.LeaveGroup %v on %v failed: %v", gaddr, ifi, err)
			}
		}
	}
}

func TestUDPMultiplePacketConnWithMultipleGroupListeners(t *testing.T) {
	switch runtime.GOOS {
	case "nacl", "plan9", "solaris", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if testing.Short() {
		t.Skip("to avoid external network")
	}

	for _, gaddr := range udpMultipleGroupListenerTests {
		c1, err := net.ListenPacket("udp4", "224.0.0.0:1024") // wildcard address with reusable port
		if err != nil {
			t.Fatalf("net.ListenPacket failed: %v", err)
		}
		defer c1.Close()

		c2, err := net.ListenPacket("udp4", "224.0.0.0:1024") // wildcard address with reusable port
		if err != nil {
			t.Fatalf("net.ListenPacket failed: %v", err)
		}
		defer c2.Close()

		var ps [2]*ipv4.PacketConn
		ps[0] = ipv4.NewPacketConn(c1)
		ps[1] = ipv4.NewPacketConn(c2)
		var mift []*net.Interface

		ift, err := net.Interfaces()
		if err != nil {
			t.Fatalf("net.Interfaces failed: %v", err)
		}
		for i, ifi := range ift {
			if _, ok := nettest.IsMulticastCapable("ip4", &ifi); !ok {
				continue
			}
			for _, p := range ps {
				if err := p.JoinGroup(&ifi, gaddr); err != nil {
					t.Fatalf("ipv4.PacketConn.JoinGroup %v on %v failed: %v", gaddr, ifi, err)
				}
			}
			mift = append(mift, &ift[i])
		}
		for _, ifi := range mift {
			for _, p := range ps {
				if err := p.LeaveGroup(ifi, gaddr); err != nil {
					t.Fatalf("ipv4.PacketConn.LeaveGroup %v on %v failed: %v", gaddr, ifi, err)
				}
			}
		}
	}
}

func TestUDPPerInterfaceSinglePacketConnWithSingleGroupListener(t *testing.T) {
	switch runtime.GOOS {
	case "nacl", "plan9", "solaris", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if testing.Short() {
		t.Skip("to avoid external network")
	}

	gaddr := net.IPAddr{IP: net.IPv4(224, 0, 0, 254)} // see RFC 4727
	type ml struct {
		c   *ipv4.PacketConn
		ifi *net.Interface
	}
	var mlt []*ml

	ift, err := net.Interfaces()
	if err != nil {
		t.Fatalf("net.Interfaces failed: %v", err)
	}
	for i, ifi := range ift {
		ip, ok := nettest.IsMulticastCapable("ip4", &ifi)
		if !ok {
			continue
		}
		c, err := net.ListenPacket("udp4", ip.String()+":"+"1024") // unicast address with non-reusable port
		if err != nil {
			t.Fatalf("net.ListenPacket with %v failed: %v", ip, err)
		}
		defer c.Close()
		p := ipv4.NewPacketConn(c)
		if err := p.JoinGroup(&ifi, &gaddr); err != nil {
			t.Fatalf("ipv4.PacketConn.JoinGroup on %v failed: %v", ifi, err)
		}
		mlt = append(mlt, &ml{p, &ift[i]})
	}
	for _, m := range mlt {
		if err := m.c.LeaveGroup(m.ifi, &gaddr); err != nil {
			t.Fatalf("ipv4.PacketConn.LeaveGroup on %v failed: %v", m.ifi, err)
		}
	}
}

func TestIPSingleRawConnWithSingleGroupListener(t *testing.T) {
	switch runtime.GOOS {
	case "nacl", "plan9", "solaris", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if testing.Short() {
		t.Skip("to avoid external network")
	}
	if os.Getuid() != 0 {
		t.Skip("must be root")
	}

	c, err := net.ListenPacket("ip4:icmp", "0.0.0.0") // wildcard address
	if err != nil {
		t.Fatalf("net.ListenPacket failed: %v", err)
	}
	defer c.Close()

	r, err := ipv4.NewRawConn(c)
	if err != nil {
		t.Fatalf("ipv4.RawConn failed: %v", err)
	}
	gaddr := net.IPAddr{IP: net.IPv4(224, 0, 0, 254)} // see RFC 4727
	var mift []*net.Interface

	ift, err := net.Interfaces()
	if err != nil {
		t.Fatalf("net.Interfaces failed: %v", err)
	}
	for i, ifi := range ift {
		if _, ok := nettest.IsMulticastCapable("ip4", &ifi); !ok {
			continue
		}
		if err := r.JoinGroup(&ifi, &gaddr); err != nil {
			t.Fatalf("ipv4.RawConn.JoinGroup on %v failed: %v", ifi, err)
		}
		mift = append(mift, &ift[i])
	}
	for _, ifi := range mift {
		if err := r.LeaveGroup(ifi, &gaddr); err != nil {
			t.Fatalf("ipv4.RawConn.LeaveGroup on %v failed: %v", ifi, err)
		}
	}
}

func TestIPPerInterfaceSingleRawConnWithSingleGroupListener(t *testing.T) {
	switch runtime.GOOS {
	case "nacl", "plan9", "solaris", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if testing.Short() {
		t.Skip("to avoid external network")
	}
	if os.Getuid() != 0 {
		t.Skip("must be root")
	}

	gaddr := net.IPAddr{IP: net.IPv4(224, 0, 0, 254)} // see RFC 4727
	type ml struct {
		c   *ipv4.RawConn
		ifi *net.Interface
	}
	var mlt []*ml

	ift, err := net.Interfaces()
	if err != nil {
		t.Fatalf("net.Interfaces failed: %v", err)
	}
	for i, ifi := range ift {
		ip, ok := nettest.IsMulticastCapable("ip4", &ifi)
		if !ok {
			continue
		}
		c, err := net.ListenPacket("ip4:253", ip.String()) // unicast address
		if err != nil {
			t.Fatalf("net.ListenPacket with %v failed: %v", ip, err)
		}
		defer c.Close()
		r, err := ipv4.NewRawConn(c)
		if err != nil {
			t.Fatalf("ipv4.NewRawConn failed: %v", err)
		}
		if err := r.JoinGroup(&ifi, &gaddr); err != nil {
			t.Fatalf("ipv4.RawConn.JoinGroup on %v failed: %v", ifi, err)
		}
		mlt = append(mlt, &ml{r, &ift[i]})
	}
	for _, m := range mlt {
		if err := m.c.LeaveGroup(m.ifi, &gaddr); err != nil {
			t.Fatalf("ipv4.RawConn.LeaveGroup on %v failed: %v", m.ifi, err)
		}
	}
}
