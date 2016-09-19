// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv4_test

import (
	"net"
	"os"
	"runtime"
	"testing"

	"github.com/hashicorp/go.net/internal/iana"
	"github.com/hashicorp/go.net/internal/nettest"
	"github.com/hashicorp/go.net/ipv4"
)

func TestConnUnicastSocketOptions(t *testing.T) {
	switch runtime.GOOS {
	case "nacl", "plan9", "solaris":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	ifi := nettest.RoutedInterface("ip4", net.FlagUp|net.FlagLoopback)
	if ifi == nil {
		t.Skipf("not available on %q", runtime.GOOS)
	}

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	defer ln.Close()

	done := make(chan bool)
	go acceptor(t, ln, done)

	c, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatalf("net.Dial failed: %v", err)
	}
	defer c.Close()

	testUnicastSocketOptions(t, ipv4.NewConn(c))

	<-done
}

var packetConnUnicastSocketOptionTests = []struct {
	net, proto, addr string
}{
	{"udp4", "", "127.0.0.1:0"},
	{"ip4", ":icmp", "127.0.0.1"},
}

func TestPacketConnUnicastSocketOptions(t *testing.T) {
	switch runtime.GOOS {
	case "nacl", "plan9", "solaris":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	ifi := nettest.RoutedInterface("ip4", net.FlagUp|net.FlagLoopback)
	if ifi == nil {
		t.Skipf("not available on %q", runtime.GOOS)
	}

	for _, tt := range packetConnUnicastSocketOptionTests {
		if tt.net == "ip4" && os.Getuid() != 0 {
			t.Skip("must be root")
		}
		c, err := net.ListenPacket(tt.net+tt.proto, tt.addr)
		if err != nil {
			t.Fatalf("net.ListenPacket(%q, %q) failed: %v", tt.net+tt.proto, tt.addr, err)
		}
		defer c.Close()

		testUnicastSocketOptions(t, ipv4.NewPacketConn(c))
	}
}

func TestRawConnUnicastSocketOptions(t *testing.T) {
	switch runtime.GOOS {
	case "nacl", "plan9", "solaris":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if os.Getuid() != 0 {
		t.Skip("must be root")
	}
	ifi := nettest.RoutedInterface("ip4", net.FlagUp|net.FlagLoopback)
	if ifi == nil {
		t.Skipf("not available on %q", runtime.GOOS)
	}

	c, err := net.ListenPacket("ip4:icmp", "127.0.0.1")
	if err != nil {
		t.Fatalf("net.ListenPacket failed: %v", err)
	}
	defer c.Close()

	r, err := ipv4.NewRawConn(c)
	if err != nil {
		t.Fatalf("ipv4.NewRawConn failed: %v", err)
	}

	testUnicastSocketOptions(t, r)
}

type testIPv4UnicastConn interface {
	TOS() (int, error)
	SetTOS(int) error
	TTL() (int, error)
	SetTTL(int) error
}

func testUnicastSocketOptions(t *testing.T, c testIPv4UnicastConn) {
	tos := iana.DiffServCS0 | iana.NotECNTransport
	switch runtime.GOOS {
	case "windows":
		// IP_TOS option is supported on Windows 8 and beyond.
		t.Skipf("skipping IP_TOS test on %q", runtime.GOOS)
	}

	if err := c.SetTOS(tos); err != nil {
		t.Fatalf("ipv4.Conn.SetTOS failed: %v", err)
	}
	if v, err := c.TOS(); err != nil {
		t.Fatalf("ipv4.Conn.TOS failed: %v", err)
	} else if v != tos {
		t.Fatalf("got unexpected TOS value %v; expected %v", v, tos)
	}
	const ttl = 255
	if err := c.SetTTL(ttl); err != nil {
		t.Fatalf("ipv4.Conn.SetTTL failed: %v", err)
	}
	if v, err := c.TTL(); err != nil {
		t.Fatalf("ipv4.Conn.TTL failed: %v", err)
	} else if v != ttl {
		t.Fatalf("got unexpected TTL value %v; expected %v", v, ttl)
	}
}
