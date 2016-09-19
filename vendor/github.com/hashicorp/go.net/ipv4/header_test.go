// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv4

import (
	"bytes"
	"net"
	"reflect"
	"runtime"
	"testing"
)

var (
	wireHeaderFromKernel = [HeaderLen]byte{
		0x45, 0x01, 0xbe, 0xef,
		0xca, 0xfe, 0x45, 0xdc,
		0xff, 0x01, 0xde, 0xad,
		172, 16, 254, 254,
		192, 168, 0, 1,
	}
	wireHeaderToKernel = [HeaderLen]byte{
		0x45, 0x01, 0xbe, 0xef,
		0xca, 0xfe, 0x45, 0xdc,
		0xff, 0x01, 0xde, 0xad,
		172, 16, 254, 254,
		192, 168, 0, 1,
	}
	wireHeaderFromTradBSDKernel = [HeaderLen]byte{
		0x45, 0x01, 0xdb, 0xbe,
		0xca, 0xfe, 0xdc, 0x45,
		0xff, 0x01, 0xde, 0xad,
		172, 16, 254, 254,
		192, 168, 0, 1,
	}
	wireHeaderFromFreeBSD10Kernel = [HeaderLen]byte{
		0x45, 0x01, 0xef, 0xbe,
		0xca, 0xfe, 0xdc, 0x45,
		0xff, 0x01, 0xde, 0xad,
		172, 16, 254, 254,
		192, 168, 0, 1,
	}
	wireHeaderToTradBSDKernel = [HeaderLen]byte{
		0x45, 0x01, 0xef, 0xbe,
		0xca, 0xfe, 0xdc, 0x45,
		0xff, 0x01, 0xde, 0xad,
		172, 16, 254, 254,
		192, 168, 0, 1,
	}
	// TODO(mikio): Add platform dependent wire header formats when
	// we support new platforms.

	testHeader = &Header{
		Version:  Version,
		Len:      HeaderLen,
		TOS:      1,
		TotalLen: 0xbeef,
		ID:       0xcafe,
		Flags:    DontFragment,
		FragOff:  1500,
		TTL:      255,
		Protocol: 1,
		Checksum: 0xdead,
		Src:      net.IPv4(172, 16, 254, 254),
		Dst:      net.IPv4(192, 168, 0, 1),
	}
)

func TestMarshalHeader(t *testing.T) {
	b, err := testHeader.Marshal()
	if err != nil {
		t.Fatalf("ipv4.Header.Marshal failed: %v", err)
	}
	var wh []byte
	if supportsNewIPInput {
		wh = wireHeaderToKernel[:]
	} else {
		wh = wireHeaderToTradBSDKernel[:]
	}
	if !bytes.Equal(b, wh) {
		t.Fatalf("ipv4.Header.Marshal failed: %#v not equal %#v", b, wh)
	}
}

func TestParseHeader(t *testing.T) {
	var wh []byte
	if supportsNewIPInput {
		wh = wireHeaderFromKernel[:]
	} else {
		if runtime.GOOS == "freebsd" && freebsdVersion >= 1000000 {
			wh = wireHeaderFromFreeBSD10Kernel[:]
		} else {
			wh = wireHeaderFromTradBSDKernel[:]
		}
	}
	h, err := ParseHeader(wh)
	if err != nil {
		t.Fatalf("ipv4.ParseHeader failed: %v", err)
	}
	if !reflect.DeepEqual(h, testHeader) {
		t.Fatalf("ipv4.ParseHeader failed: %#v not equal %#v", h, testHeader)
	}
}
