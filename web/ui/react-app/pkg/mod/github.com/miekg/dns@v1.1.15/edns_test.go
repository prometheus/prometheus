package dns

import (
	"net"
	"testing"
)

func TestOPTTtl(t *testing.T) {
	e := &OPT{}
	e.Hdr.Name = "."
	e.Hdr.Rrtype = TypeOPT

	// verify the default setting of DO=0
	if e.Do() {
		t.Errorf("DO bit should be zero")
	}

	// There are 6 possible invocations of SetDo():
	//
	// 1. Starting with DO=0, using SetDo()
	// 2. Starting with DO=0, using SetDo(true)
	// 3. Starting with DO=0, using SetDo(false)
	// 4. Starting with DO=1, using SetDo()
	// 5. Starting with DO=1, using SetDo(true)
	// 6. Starting with DO=1, using SetDo(false)

	// verify that invoking SetDo() sets DO=1 (TEST #1)
	e.SetDo()
	if !e.Do() {
		t.Errorf("DO bit should be non-zero")
	}
	// verify that using SetDo(true) works when DO=1 (TEST #5)
	e.SetDo(true)
	if !e.Do() {
		t.Errorf("DO bit should still be non-zero")
	}
	// verify that we can use SetDo(false) to set DO=0 (TEST #6)
	e.SetDo(false)
	if e.Do() {
		t.Errorf("DO bit should be zero")
	}
	// verify that if we call SetDo(false) when DO=0 that it is unchanged (TEST #3)
	e.SetDo(false)
	if e.Do() {
		t.Errorf("DO bit should still be zero")
	}
	// verify that using SetDo(true) works for DO=0 (TEST #2)
	e.SetDo(true)
	if !e.Do() {
		t.Errorf("DO bit should be non-zero")
	}
	// verify that using SetDo() works for DO=1 (TEST #4)
	e.SetDo()
	if !e.Do() {
		t.Errorf("DO bit should be non-zero")
	}

	if e.Version() != 0 {
		t.Errorf("version should be non-zero")
	}

	e.SetVersion(42)
	if e.Version() != 42 {
		t.Errorf("set 42, expected %d, got %d", 42, e.Version())
	}

	e.SetExtendedRcode(42)
	// ExtendedRcode has the last 4 bits set to 0.
	if e.ExtendedRcode() != 42&0xFFFFFFF0 {
		t.Errorf("set 42, expected %d, got %d", 42&0xFFFFFFF0, e.ExtendedRcode())
	}

	// This will reset the 8 upper bits of the extended rcode
	e.SetExtendedRcode(RcodeNotAuth)
	if e.ExtendedRcode() != 0 {
		t.Errorf("Setting a non-extended rcode is expected to set extended rcode to 0, got: %d", e.ExtendedRcode())
	}
}

func TestEDNS0_SUBNETUnpack(t *testing.T) {
	for _, ip := range []net.IP{
		net.IPv4(0xde, 0xad, 0xbe, 0xef),
		net.ParseIP("192.0.2.1"),
		net.ParseIP("2001:db8::68"),
	} {
		var s1 EDNS0_SUBNET
		s1.Address = ip

		if ip.To4() == nil {
			s1.Family = 2
			s1.SourceNetmask = net.IPv6len * 8
		} else {
			s1.Family = 1
			s1.SourceNetmask = net.IPv4len * 8
		}

		b, err := s1.pack()
		if err != nil {
			t.Fatalf("failed to pack: %v", err)
		}

		var s2 EDNS0_SUBNET
		if err := s2.unpack(b); err != nil {
			t.Fatalf("failed to unpack: %v", err)
		}

		if !ip.Equal(s2.Address) {
			t.Errorf("address different after unpacking; expected %s, got %s", ip, s2.Address)
		}
	}
}
