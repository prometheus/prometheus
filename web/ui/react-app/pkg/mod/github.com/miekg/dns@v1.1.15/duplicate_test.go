package dns

import "testing"

func TestDuplicateA(t *testing.T) {
	a1, _ := NewRR("www.example.org. 2700 IN A 127.0.0.1")
	a2, _ := NewRR("www.example.org. IN A 127.0.0.1")
	if !IsDuplicate(a1, a2) {
		t.Errorf("expected %s/%s to be duplicates, but got false", a1.String(), a2.String())
	}

	a2, _ = NewRR("www.example.org. IN A 127.0.0.2")
	if IsDuplicate(a1, a2) {
		t.Errorf("expected %s/%s not to be duplicates, but got true", a1.String(), a2.String())
	}
}

func TestDuplicateTXT(t *testing.T) {
	a1, _ := NewRR("www.example.org. IN TXT \"aa\"")
	a2, _ := NewRR("www.example.org. IN TXT \"aa\"")

	if !IsDuplicate(a1, a2) {
		t.Errorf("expected %s/%s to be duplicates, but got false", a1.String(), a2.String())
	}

	a2, _ = NewRR("www.example.org. IN TXT \"aa\" \"bb\"")
	if IsDuplicate(a1, a2) {
		t.Errorf("expected %s/%s not to be duplicates, but got true", a1.String(), a2.String())
	}

	a1, _ = NewRR("www.example.org. IN TXT \"aa\" \"bc\"")
	if IsDuplicate(a1, a2) {
		t.Errorf("expected %s/%s not to be duplicates, but got true", a1.String(), a2.String())
	}
}

func TestDuplicateOwner(t *testing.T) {
	a1, _ := NewRR("www.example.org. IN A 127.0.0.1")
	a2, _ := NewRR("www.example.org. IN A 127.0.0.1")
	if !IsDuplicate(a1, a2) {
		t.Errorf("expected %s/%s to be duplicates, but got false", a1.String(), a2.String())
	}

	a2, _ = NewRR("WWw.exaMPle.org. IN A 127.0.0.2")
	if IsDuplicate(a1, a2) {
		t.Errorf("expected %s/%s to be duplicates, but got false", a1.String(), a2.String())
	}
}

func TestDuplicateDomain(t *testing.T) {
	a1, _ := NewRR("www.example.org. IN CNAME example.org.")
	a2, _ := NewRR("www.example.org. IN CNAME example.org.")
	if !IsDuplicate(a1, a2) {
		t.Errorf("expected %s/%s to be duplicates, but got false", a1.String(), a2.String())
	}

	a2, _ = NewRR("www.example.org. IN CNAME exAMPLe.oRG.")
	if !IsDuplicate(a1, a2) {
		t.Errorf("expected %s/%s to be duplicates, but got false", a1.String(), a2.String())
	}
}

func TestDuplicateWrongRrtype(t *testing.T) {
	// Test that IsDuplicate won't panic for a record that's lying about
	// it's Rrtype.

	r1 := &A{Hdr: RR_Header{Rrtype: TypeA}}
	r2 := &AAAA{Hdr: RR_Header{Rrtype: TypeA}}
	if IsDuplicate(r1, r2) {
		t.Errorf("expected %s/%s not to be duplicates, but got true", r1.String(), r2.String())
	}

	r3 := &AAAA{Hdr: RR_Header{Rrtype: TypeA}}
	r4 := &A{Hdr: RR_Header{Rrtype: TypeA}}
	if IsDuplicate(r3, r4) {
		t.Errorf("expected %s/%s not to be duplicates, but got true", r3.String(), r4.String())
	}

	r5 := &AAAA{Hdr: RR_Header{Rrtype: TypeA}}
	r6 := &AAAA{Hdr: RR_Header{Rrtype: TypeA}}
	if !IsDuplicate(r5, r6) {
		t.Errorf("expected %s/%s to be duplicates, but got false", r5.String(), r6.String())
	}
}
