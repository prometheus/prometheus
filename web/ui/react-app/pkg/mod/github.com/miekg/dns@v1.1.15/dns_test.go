package dns

import (
	"bytes"
	"encoding/hex"
	"net"
	"testing"
)

func TestPackUnpack(t *testing.T) {
	out := new(Msg)
	out.Answer = make([]RR, 1)
	key := &DNSKEY{Flags: 257, Protocol: 3, Algorithm: RSASHA1}
	key.Hdr = RR_Header{Name: "miek.nl.", Rrtype: TypeDNSKEY, Class: ClassINET, Ttl: 3600}
	key.PublicKey = "AwEAAaHIwpx3w4VHKi6i1LHnTaWeHCL154Jug0Rtc9ji5qwPXpBo6A5sRv7cSsPQKPIwxLpyCrbJ4mr2L0EPOdvP6z6YfljK2ZmTbogU9aSU2fiq/4wjxbdkLyoDVgtO+JsxNN4bjr4WcWhsmk1Hg93FV9ZpkWb0Tbad8DFqNDzr//kZ"

	out.Answer[0] = key
	msg, err := out.Pack()
	if err != nil {
		t.Error("failed to pack msg with DNSKEY")
	}
	in := new(Msg)
	if in.Unpack(msg) != nil {
		t.Error("failed to unpack msg with DNSKEY")
	}

	sig := &RRSIG{TypeCovered: TypeDNSKEY, Algorithm: RSASHA1, Labels: 2,
		OrigTtl: 3600, Expiration: 4000, Inception: 4000, KeyTag: 34641, SignerName: "miek.nl.",
		Signature: "AwEAAaHIwpx3w4VHKi6i1LHnTaWeHCL154Jug0Rtc9ji5qwPXpBo6A5sRv7cSsPQKPIwxLpyCrbJ4mr2L0EPOdvP6z6YfljK2ZmTbogU9aSU2fiq/4wjxbdkLyoDVgtO+JsxNN4bjr4WcWhsmk1Hg93FV9ZpkWb0Tbad8DFqNDzr//kZ"}
	sig.Hdr = RR_Header{Name: "miek.nl.", Rrtype: TypeRRSIG, Class: ClassINET, Ttl: 3600}

	out.Answer[0] = sig
	msg, err = out.Pack()
	if err != nil {
		t.Error("failed to pack msg with RRSIG")
	}

	if in.Unpack(msg) != nil {
		t.Error("failed to unpack msg with RRSIG")
	}
}

func TestPackUnpack2(t *testing.T) {
	m := new(Msg)
	m.Extra = make([]RR, 1)
	m.Answer = make([]RR, 1)
	dom := "miek.nl."
	rr := new(A)
	rr.Hdr = RR_Header{Name: dom, Rrtype: TypeA, Class: ClassINET, Ttl: 0}
	rr.A = net.IPv4(127, 0, 0, 1)

	x := new(TXT)
	x.Hdr = RR_Header{Name: dom, Rrtype: TypeTXT, Class: ClassINET, Ttl: 0}
	x.Txt = []string{"heelalaollo"}

	m.Extra[0] = x
	m.Answer[0] = rr
	_, err := m.Pack()
	if err != nil {
		t.Error("Packing failed: ", err)
		return
	}
}

func TestPackUnpack3(t *testing.T) {
	m := new(Msg)
	m.Extra = make([]RR, 2)
	m.Answer = make([]RR, 1)
	dom := "miek.nl."
	rr := new(A)
	rr.Hdr = RR_Header{Name: dom, Rrtype: TypeA, Class: ClassINET, Ttl: 0}
	rr.A = net.IPv4(127, 0, 0, 1)

	x1 := new(TXT)
	x1.Hdr = RR_Header{Name: dom, Rrtype: TypeTXT, Class: ClassINET, Ttl: 0}
	x1.Txt = []string{}

	x2 := new(TXT)
	x2.Hdr = RR_Header{Name: dom, Rrtype: TypeTXT, Class: ClassINET, Ttl: 0}
	x2.Txt = []string{"heelalaollo"}

	m.Extra[0] = x1
	m.Extra[1] = x2
	m.Answer[0] = rr
	b, err := m.Pack()
	if err != nil {
		t.Error("packing failed: ", err)
		return
	}

	var unpackMsg Msg
	err = unpackMsg.Unpack(b)
	if err != nil {
		t.Error("unpacking failed")
		return
	}
}

func TestBailiwick(t *testing.T) {
	yes := map[string]string{
		"miek1.nl": "miek1.nl",
		"miek.nl":  "ns.miek.nl",
		".":        "miek.nl",
	}
	for parent, child := range yes {
		if !IsSubDomain(parent, child) {
			t.Errorf("%s should be child of %s", child, parent)
			t.Errorf("comparelabels %d", CompareDomainName(parent, child))
			t.Errorf("lenlabels %d %d", CountLabel(parent), CountLabel(child))
		}
	}
	no := map[string]string{
		"www.miek.nl":  "ns.miek.nl",
		"m\\.iek.nl":   "ns.miek.nl",
		"w\\.iek.nl":   "w.iek.nl",
		"p\\\\.iek.nl": "ns.p.iek.nl", // p\\.iek.nl , literal \ in domain name
		"miek.nl":      ".",
	}
	for parent, child := range no {
		if IsSubDomain(parent, child) {
			t.Errorf("%s should not be child of %s", child, parent)
			t.Errorf("comparelabels %d", CompareDomainName(parent, child))
			t.Errorf("lenlabels %d %d", CountLabel(parent), CountLabel(child))
		}
	}
}

func TestPackNAPTR(t *testing.T) {
	for _, n := range []string{
		`apple.com. IN NAPTR   100 50 "se" "SIP+D2U" "" _sip._udp.apple.com.`,
		`apple.com. IN NAPTR   90 50 "se" "SIP+D2T" "" _sip._tcp.apple.com.`,
		`apple.com. IN NAPTR   50 50 "se" "SIPS+D2T" "" _sips._tcp.apple.com.`,
	} {
		rr := testRR(n)
		msg := make([]byte, Len(rr))
		if off, err := PackRR(rr, msg, 0, nil, false); err != nil {
			t.Errorf("packing failed: %v", err)
			t.Errorf("length %d, need more than %d", Len(rr), off)
		}
	}
}

func TestToRFC3597(t *testing.T) {
	a := testRR("miek.nl. IN A 10.0.1.1")
	x := new(RFC3597)
	x.ToRFC3597(a)
	if x.String() != `miek.nl.	3600	CLASS1	TYPE1	\# 4 0a000101` {
		t.Errorf("string mismatch, got: %s", x)
	}

	b := testRR("miek.nl. IN MX 10 mx.miek.nl.")
	x.ToRFC3597(b)
	if x.String() != `miek.nl.	3600	CLASS1	TYPE15	\# 14 000a026d78046d69656b026e6c00` {
		t.Errorf("string mismatch, got: %s", x)
	}
}

func TestNoRdataPack(t *testing.T) {
	data := make([]byte, 1024)
	for typ, fn := range TypeToRR {
		r := fn()
		*r.Header() = RR_Header{Name: "miek.nl.", Rrtype: typ, Class: ClassINET, Ttl: 16}
		_, err := PackRR(r, data, 0, nil, false)
		if err != nil {
			t.Errorf("failed to pack RR with zero rdata: %s: %v", TypeToString[typ], err)
		}
	}
}

func TestNoRdataUnpack(t *testing.T) {
	data := make([]byte, 1024)
	for typ, fn := range TypeToRR {
		if typ == TypeSOA || typ == TypeTSIG || typ == TypeTKEY {
			// SOA, TSIG will not be seen (like this) in dyn. updates?
			// TKEY requires length fields to be present for the Key and OtherData fields
			continue
		}
		r := fn()
		*r.Header() = RR_Header{Name: "miek.nl.", Rrtype: typ, Class: ClassINET, Ttl: 16}
		off, err := PackRR(r, data, 0, nil, false)
		if err != nil {
			// Should always works, TestNoDataPack should have caught this
			t.Errorf("failed to pack RR: %v", err)
			continue
		}
		if _, _, err := UnpackRR(data[:off], 0); err != nil {
			t.Errorf("failed to unpack RR with zero rdata: %s: %v", TypeToString[typ], err)
		}
	}
}

func TestRdataOverflow(t *testing.T) {
	rr := new(RFC3597)
	rr.Hdr.Name = "."
	rr.Hdr.Class = ClassINET
	rr.Hdr.Rrtype = 65280
	rr.Rdata = hex.EncodeToString(make([]byte, 0xFFFF))
	buf := make([]byte, 0xFFFF*2)
	if _, err := PackRR(rr, buf, 0, nil, false); err != nil {
		t.Fatalf("maximum size rrdata pack failed: %v", err)
	}
	rr.Rdata += "00"
	if _, err := PackRR(rr, buf, 0, nil, false); err != ErrRdata {
		t.Fatalf("oversize rrdata pack didn't return ErrRdata - instead: %v", err)
	}
}

func TestCopy(t *testing.T) {
	rr := testRR("miek.nl. 2311 IN A 127.0.0.1") // Weird TTL to avoid catching TTL
	rr1 := Copy(rr)
	if rr.String() != rr1.String() {
		t.Fatalf("Copy() failed %s != %s", rr.String(), rr1.String())
	}
}

func TestMsgCopy(t *testing.T) {
	m := new(Msg)
	m.SetQuestion("miek.nl.", TypeA)
	rr := testRR("miek.nl. 2311 IN A 127.0.0.1")
	m.Answer = []RR{rr}
	rr = testRR("miek.nl. 2311 IN NS 127.0.0.1")
	m.Ns = []RR{rr}

	m1 := m.Copy()
	if m.String() != m1.String() {
		t.Fatalf("Msg.Copy() failed %s != %s", m.String(), m1.String())
	}

	m1.Answer[0] = testRR("somethingelse.nl. 2311 IN A 127.0.0.1")
	if m.String() == m1.String() {
		t.Fatalf("Msg.Copy() failed; change to copy changed template %s", m.String())
	}

	rr = testRR("miek.nl. 2311 IN A 127.0.0.2")
	m1.Answer = append(m1.Answer, rr)
	if m1.Ns[0].String() == m1.Answer[1].String() {
		t.Fatalf("Msg.Copy() failed; append changed underlying array %s", m1.Ns[0].String())
	}
}

func TestMsgPackBuffer(t *testing.T) {
	var testMessages = []string{
		// news.ycombinator.com.in.escapemg.com.	IN	A, response
		"586285830001000000010000046e6577730b79636f6d62696e61746f7203636f6d02696e086573636170656d6703636f6d0000010001c0210006000100000e10002c036e7332c02103646e730b67726f6f7665736861726bc02d77ed50e600002a3000000e1000093a8000000e10",

		// news.ycombinator.com.in.escapemg.com.	IN	A, question
		"586201000001000000000000046e6577730b79636f6d62696e61746f7203636f6d02696e086573636170656d6703636f6d0000010001",

		"398781020001000000000000046e6577730b79636f6d62696e61746f7203636f6d0000010001",
	}

	for i, hexData := range testMessages {
		// we won't fail the decoding of the hex
		input, _ := hex.DecodeString(hexData)
		m := new(Msg)
		if err := m.Unpack(input); err != nil {
			t.Errorf("packet %d failed to unpack", i)
			continue
		}
	}
}

// Make sure we can decode a TKEY packet from the string, modify the RR, and then pack it again.
func TestTKEY(t *testing.T) {
	// An example TKEY RR captured.  There is no known accepted standard text format for a TKEY
	// record so we do this from a hex string instead of from a text readable string.
	tkeyStr := "0737362d6d732d370932322d3332633233332463303439663961662d633065612d313165372d363839362d6463333937396666656666640000f900ff0000000000d2086773732d747369670059fd01f359fe53730003000000b8a181b53081b2a0030a0100a10b06092a864882f712010202a2819d04819a60819706092a864886f71201020202006f8187308184a003020105a10302010fa2783076a003020112a26f046db29b1b1d2625da3b20b49dafef930dd1e9aad335e1c5f45dcd95e0005d67a1100f3e573d70506659dbed064553f1ab890f68f65ae10def0dad5b423b39f240ebe666f2886c5fe03819692d29182bbed87b83e1f9d16b7334ec16a3c4fc5ad4a990088e0be43f0c6957916f5fe60000"
	tkeyBytes, err := hex.DecodeString(tkeyStr)
	if err != nil {
		t.Fatal("unable to decode TKEY string ", err)
	}
	// Decode the RR
	rr, tkeyLen, unPackErr := UnpackRR(tkeyBytes, 0)
	if unPackErr != nil {
		t.Fatal("unable to decode TKEY RR", unPackErr)
	}
	// Make sure it's a TKEY record
	if rr.Header().Rrtype != TypeTKEY {
		t.Fatal("Unable to decode TKEY")
	}
	// Make sure we get back the same length
	if Len(rr) != len(tkeyBytes) {
		t.Fatalf("Lengths don't match %d != %d", Len(rr), len(tkeyBytes))
	}
	// make space for it with some fudge room
	msg := make([]byte, tkeyLen+1000)
	offset, packErr := PackRR(rr, msg, 0, nil, false)
	if packErr != nil {
		t.Fatal("unable to pack TKEY RR", packErr)
	}
	if offset != len(tkeyBytes) {
		t.Fatalf("mismatched TKEY RR size %d != %d", len(tkeyBytes), offset)
	}
	if !bytes.Equal(tkeyBytes, msg[0:offset]) {
		t.Fatal("mismatched TKEY data after rewriting bytes")
	}

	// Now add some bytes to this and make sure we can encode OtherData properly
	tkey := rr.(*TKEY)
	tkey.OtherData = "abcd"
	tkey.OtherLen = 2
	offset, packErr = PackRR(tkey, msg, 0, nil, false)
	if packErr != nil {
		t.Fatal("unable to pack TKEY RR after modification", packErr)
	}
	if offset != len(tkeyBytes)+2 {
		t.Fatalf("mismatched TKEY RR size %d != %d", offset, len(tkeyBytes)+2)
	}

	// Make sure we can parse our string output
	tkey.Hdr.Class = ClassINET // https://github.com/miekg/dns/issues/577
	_, newError := NewRR(tkey.String())
	if newError != nil {
		t.Fatalf("unable to parse TKEY string: %s", newError)
	}
}
