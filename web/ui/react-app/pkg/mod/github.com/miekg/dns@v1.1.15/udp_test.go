// +build linux,!appengine

package dns

import (
	"bytes"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

func TestSetUDPSocketOptions(t *testing.T) {
	// returns an error if we cannot resolve that address
	testFamily := func(n, addr string) error {
		a, err := net.ResolveUDPAddr(n, addr)
		if err != nil {
			return err
		}
		c, err := net.ListenUDP(n, a)
		if err != nil {
			return err
		}
		if err := setUDPSocketOptions(c); err != nil {
			t.Fatalf("failed to set socket options: %v", err)
		}
		ch := make(chan *SessionUDP)
		go func() {
			// Set some deadline so this goroutine doesn't hang forever
			c.SetReadDeadline(time.Now().Add(time.Minute))
			b := make([]byte, 1)
			_, sess, err := ReadFromSessionUDP(c, b)
			if err != nil {
				t.Errorf("failed to read from conn: %v", err)
				// fallthrough to chan send below
			}
			ch <- sess
		}()

		c2, err := net.Dial("udp", c.LocalAddr().String())
		if err != nil {
			t.Fatalf("failed to dial udp: %v", err)
		}
		if _, err := c2.Write([]byte{1}); err != nil {
			t.Fatalf("failed to write to conn: %v", err)
		}
		sess := <-ch
		if sess == nil {
			// t.Error was already called in the goroutine above.
			t.FailNow()
		}
		if len(sess.context) == 0 {
			t.Fatalf("empty session context: %v", sess)
		}
		ip := parseDstFromOOB(sess.context)
		if ip == nil {
			t.Fatalf("failed to parse dst: %v", sess)
		}
		if !strings.Contains(c.LocalAddr().String(), ip.String()) {
			t.Fatalf("dst was different than listen addr: %v != %v", ip.String(), c.LocalAddr().String())
		}
		return nil
	}

	// we require that ipv4 be supported
	if err := testFamily("udp4", "127.0.0.1:0"); err != nil {
		t.Fatalf("failed to test socket options on IPv4: %v", err)
	}
	// IPv6 might not be supported so these will just log
	if err := testFamily("udp6", "[::1]:0"); err != nil {
		t.Logf("failed to test socket options on IPv6-only: %v", err)
	}
	if err := testFamily("udp", "[::1]:0"); err != nil {
		t.Logf("failed to test socket options on IPv6/IPv4: %v", err)
	}
}

func TestParseDstFromOOB(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		// The cmsghdr struct differs in the width (32/64-bit) of
		// lengths and the struct padding between architectures.
		// The data below was only written with amd64 in mind, and
		// thus the test must be skipped on other architectures.
		t.Skip("skipping test on unsupported architecture")
	}

	// dst is :ffff:100.100.100.100
	oob := []byte{36, 0, 0, 0, 0, 0, 0, 0, 41, 0, 0, 0, 50, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 100, 100, 100, 100, 2, 0, 0, 0}
	dst := parseDstFromOOB(oob)
	dst4 := dst.To4()
	if dst4 == nil {
		t.Errorf("failed to parse IPv4 in IPv6: %v", dst)
	} else if dst4.String() != "100.100.100.100" {
		t.Errorf("unexpected IPv4: %v", dst4)
	}

	// dst is 2001:db8::1
	oob = []byte{36, 0, 0, 0, 0, 0, 0, 0, 41, 0, 0, 0, 50, 0, 0, 0, 32, 1, 13, 184, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 0, 0, 0, 0, 0}
	dst = parseDstFromOOB(oob)
	dst6 := dst.To16()
	if dst6 == nil {
		t.Errorf("failed to parse IPv6: %v", dst)
	} else if dst6.String() != "2001:db8::1" {
		t.Errorf("unexpected IPv6: %v", dst4)
	}

	// dst is 100.100.100.100 but was received on 10.10.10.10
	oob = []byte{28, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 0, 0, 0, 2, 0, 0, 0, 10, 10, 10, 10, 100, 100, 100, 100, 0, 0, 0, 0}
	dst = parseDstFromOOB(oob)
	dst4 = dst.To4()
	if dst4 == nil {
		t.Errorf("failed to parse IPv4: %v", dst)
	} else if dst4.String() != "100.100.100.100" {
		t.Errorf("unexpected IPv4: %v", dst4)
	}
}

func TestCorrectSource(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		// See comment above in TestParseDstFromOOB.
		t.Skip("skipping test on unsupported architecture")
	}

	// dst is :ffff:100.100.100.100 which should be counted as IPv4
	oob := []byte{36, 0, 0, 0, 0, 0, 0, 0, 41, 0, 0, 0, 50, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 100, 100, 100, 100, 2, 0, 0, 0}
	soob := correctSource(oob)
	cm4 := new(ipv4.ControlMessage)
	cm4.Src = net.ParseIP("100.100.100.100")
	if !bytes.Equal(soob, cm4.Marshal()) {
		t.Errorf("unexpected oob for ipv4 address: %v", soob)
	}

	// dst is 2001:db8::1
	oob = []byte{36, 0, 0, 0, 0, 0, 0, 0, 41, 0, 0, 0, 50, 0, 0, 0, 32, 1, 13, 184, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 0, 0, 0, 0, 0}
	soob = correctSource(oob)
	cm6 := new(ipv6.ControlMessage)
	cm6.Src = net.ParseIP("2001:db8::1")
	if !bytes.Equal(soob, cm6.Marshal()) {
		t.Errorf("unexpected oob for IPv6 address: %v", soob)
	}
}
