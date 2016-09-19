// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv4_test

import (
	"net"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/hashicorp/go.net/internal/iana"
	"github.com/hashicorp/go.net/internal/icmp"
	"github.com/hashicorp/go.net/internal/nettest"
	"github.com/hashicorp/go.net/ipv4"
)

func TestPacketConnReadWriteMulticastUDP(t *testing.T) {
	switch runtime.GOOS {
	case "nacl", "plan9", "solaris", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	ifi := nettest.RoutedInterface("ip4", net.FlagUp|net.FlagMulticast|net.FlagLoopback)
	if ifi == nil {
		t.Skipf("not available on %q", runtime.GOOS)
	}

	c, err := net.ListenPacket("udp4", "224.0.0.0:0") // see RFC 4727
	if err != nil {
		t.Fatalf("net.ListenPacket failed: %v", err)
	}
	defer c.Close()

	_, port, err := net.SplitHostPort(c.LocalAddr().String())
	if err != nil {
		t.Fatalf("net.SplitHostPort failed: %v", err)
	}
	dst, err := net.ResolveUDPAddr("udp4", "224.0.0.254:"+port) // see RFC 4727
	if err != nil {
		t.Fatalf("net.ResolveUDPAddr failed: %v", err)
	}

	p := ipv4.NewPacketConn(c)
	defer p.Close()
	if err := p.JoinGroup(ifi, dst); err != nil {
		t.Fatalf("ipv4.PacketConn.JoinGroup on %v failed: %v", ifi, err)
	}
	if err := p.SetMulticastInterface(ifi); err != nil {
		t.Fatalf("ipv4.PacketConn.SetMulticastInterface failed: %v", err)
	}
	if _, err := p.MulticastInterface(); err != nil {
		t.Fatalf("ipv4.PacketConn.MulticastInterface failed: %v", err)
	}
	if err := p.SetMulticastLoopback(true); err != nil {
		t.Fatalf("ipv4.PacketConn.SetMulticastLoopback failed: %v", err)
	}
	if _, err := p.MulticastLoopback(); err != nil {
		t.Fatalf("ipv4.PacketConn.MulticastLoopback failed: %v", err)
	}
	cf := ipv4.FlagTTL | ipv4.FlagDst | ipv4.FlagInterface

	for i, toggle := range []bool{true, false, true} {
		if err := p.SetControlMessage(cf, toggle); err != nil {
			if protocolNotSupported(err) {
				t.Skipf("not supported on %q", runtime.GOOS)
			}
			t.Fatalf("ipv4.PacketConn.SetControlMessage failed: %v", err)
		}
		if err := p.SetDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
			t.Fatalf("ipv4.PacketConn.SetDeadline failed: %v", err)
		}
		p.SetMulticastTTL(i + 1)
		if _, err := p.WriteTo([]byte("HELLO-R-U-THERE"), nil, dst); err != nil {
			t.Fatalf("ipv4.PacketConn.WriteTo failed: %v", err)
		}
		b := make([]byte, 128)
		if _, cm, _, err := p.ReadFrom(b); err != nil {
			t.Fatalf("ipv4.PacketConn.ReadFrom failed: %v", err)
		} else {
			t.Logf("rcvd cmsg: %v", cm)
		}
	}
}

func TestPacketConnReadWriteMulticastICMP(t *testing.T) {
	switch runtime.GOOS {
	case "nacl", "plan9", "solaris", "windows":
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

	dst, err := net.ResolveIPAddr("ip4", "224.0.0.254") // see RFC 4727
	if err != nil {
		t.Fatalf("net.ResolveIPAddr failed: %v", err)
	}

	p := ipv4.NewPacketConn(c)
	defer p.Close()
	if err := p.JoinGroup(ifi, dst); err != nil {
		t.Fatalf("ipv4.PacketConn.JoinGroup on %v failed: %v", ifi, err)
	}
	if err := p.SetMulticastInterface(ifi); err != nil {
		t.Fatalf("ipv4.PacketConn.SetMulticastInterface failed: %v", err)
	}
	if _, err := p.MulticastInterface(); err != nil {
		t.Fatalf("ipv4.PacketConn.MulticastInterface failed: %v", err)
	}
	if err := p.SetMulticastLoopback(true); err != nil {
		t.Fatalf("ipv4.PacketConn.SetMulticastLoopback failed: %v", err)
	}
	if _, err := p.MulticastLoopback(); err != nil {
		t.Fatalf("ipv4.PacketConn.MulticastLoopback failed: %v", err)
	}
	cf := ipv4.FlagTTL | ipv4.FlagDst | ipv4.FlagInterface

	for i, toggle := range []bool{true, false, true} {
		wb, err := (&icmp.Message{
			Type: ipv4.ICMPTypeEcho, Code: 0,
			Body: &icmp.Echo{
				ID: os.Getpid() & 0xffff, Seq: i + 1,
				Data: []byte("HELLO-R-U-THERE"),
			},
		}).Marshal(nil)
		if err != nil {
			t.Fatalf("icmp.Message.Marshal failed: %v", err)
		}
		if err := p.SetControlMessage(cf, toggle); err != nil {
			if protocolNotSupported(err) {
				t.Skipf("not supported on %q", runtime.GOOS)
			}
			t.Fatalf("ipv4.PacketConn.SetControlMessage failed: %v", err)
		}
		if err := p.SetDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
			t.Fatalf("ipv4.PacketConn.SetDeadline failed: %v", err)
		}
		p.SetMulticastTTL(i + 1)
		if _, err := p.WriteTo(wb, nil, dst); err != nil {
			t.Fatalf("ipv4.PacketConn.WriteTo failed: %v", err)
		}
		b := make([]byte, 128)
		if n, cm, _, err := p.ReadFrom(b); err != nil {
			t.Fatalf("ipv4.PacketConn.ReadFrom failed: %v", err)
		} else {
			t.Logf("rcvd cmsg: %v", cm)
			m, err := icmp.ParseMessage(iana.ProtocolICMP, b[:n])
			if err != nil {
				t.Fatalf("icmp.ParseMessage failed: %v", err)
			}
			switch {
			case m.Type == ipv4.ICMPTypeEchoReply && m.Code == 0: // net.inet.icmp.bmcastecho=1
			case m.Type == ipv4.ICMPTypeEcho && m.Code == 0: // net.inet.icmp.bmcastecho=0
			default:
				t.Fatalf("got type=%v, code=%v; expected type=%v, code=%v", m.Type, m.Code, ipv4.ICMPTypeEchoReply, 0)
			}
		}
	}
}

func TestRawConnReadWriteMulticastICMP(t *testing.T) {
	if testing.Short() {
		t.Skip("to avoid external network")
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

	dst, err := net.ResolveIPAddr("ip4", "224.0.0.254") // see RFC 4727
	if err != nil {
		t.Fatalf("ResolveIPAddr failed: %v", err)
	}

	r, err := ipv4.NewRawConn(c)
	if err != nil {
		t.Fatalf("ipv4.NewRawConn failed: %v", err)
	}
	defer r.Close()
	if err := r.JoinGroup(ifi, dst); err != nil {
		t.Fatalf("ipv4.RawConn.JoinGroup on %v failed: %v", ifi, err)
	}
	if err := r.SetMulticastInterface(ifi); err != nil {
		t.Fatalf("ipv4.RawConn.SetMulticastInterface failed: %v", err)
	}
	if _, err := r.MulticastInterface(); err != nil {
		t.Fatalf("ipv4.RawConn.MulticastInterface failed: %v", err)
	}
	if err := r.SetMulticastLoopback(true); err != nil {
		t.Fatalf("ipv4.RawConn.SetMulticastLoopback failed: %v", err)
	}
	if _, err := r.MulticastLoopback(); err != nil {
		t.Fatalf("ipv4.RawConn.MulticastLoopback failed: %v", err)
	}
	cf := ipv4.FlagTTL | ipv4.FlagDst | ipv4.FlagInterface

	for i, toggle := range []bool{true, false, true} {
		wb, err := (&icmp.Message{
			Type: ipv4.ICMPTypeEcho, Code: 0,
			Body: &icmp.Echo{
				ID: os.Getpid() & 0xffff, Seq: i + 1,
				Data: []byte("HELLO-R-U-THERE"),
			},
		}).Marshal(nil)
		if err != nil {
			t.Fatalf("icmp.Message.Marshal failed: %v", err)
		}
		wh := &ipv4.Header{
			Version:  ipv4.Version,
			Len:      ipv4.HeaderLen,
			TOS:      i + 1,
			TotalLen: ipv4.HeaderLen + len(wb),
			Protocol: 1,
			Dst:      dst.IP,
		}
		if err := r.SetControlMessage(cf, toggle); err != nil {
			if protocolNotSupported(err) {
				t.Skipf("not supported on %q", runtime.GOOS)
			}
			t.Fatalf("ipv4.RawConn.SetControlMessage failed: %v", err)
		}
		if err := r.SetDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
			t.Fatalf("ipv4.RawConn.SetDeadline failed: %v", err)
		}
		r.SetMulticastTTL(i + 1)
		if err := r.WriteTo(wh, wb, nil); err != nil {
			t.Fatalf("ipv4.RawConn.WriteTo failed: %v", err)
		}
		rb := make([]byte, ipv4.HeaderLen+128)
		if rh, b, cm, err := r.ReadFrom(rb); err != nil {
			t.Fatalf("ipv4.RawConn.ReadFrom failed: %v", err)
		} else {
			t.Logf("rcvd cmsg: %v", cm)
			m, err := icmp.ParseMessage(iana.ProtocolICMP, b)
			if err != nil {
				t.Fatalf("icmp.ParseMessage failed: %v", err)
			}
			switch {
			case (rh.Dst.IsLoopback() || rh.Dst.IsLinkLocalUnicast() || rh.Dst.IsGlobalUnicast()) && m.Type == ipv4.ICMPTypeEchoReply && m.Code == 0: // net.inet.icmp.bmcastecho=1
			case rh.Dst.IsMulticast() && m.Type == ipv4.ICMPTypeEcho && m.Code == 0: // net.inet.icmp.bmcastecho=0
			default:
				t.Fatalf("got type=%v, code=%v; expected type=%v, code=%v", m.Type, m.Code, ipv4.ICMPTypeEchoReply, 0)
			}
		}
	}
}
