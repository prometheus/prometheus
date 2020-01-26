package dns

import (
	"bytes"
	"testing"
)

func TestDynamicUpdateParsing(t *testing.T) {
	prefix := "example.com. IN "
	for _, typ := range TypeToString {
		if typ == "OPT" || typ == "AXFR" || typ == "IXFR" || typ == "ANY" || typ == "TKEY" ||
			typ == "TSIG" || typ == "ISDN" || typ == "UNSPEC" || typ == "NULL" || typ == "ATMA" ||
			typ == "Reserved" || typ == "None" || typ == "NXT" || typ == "MAILB" || typ == "MAILA" {
			continue
		}
		if _, err := NewRR(prefix + typ); err != nil {
			t.Errorf("failure to parse: %s %s: %v", prefix, typ, err)
		}
	}
}

func TestDynamicUpdateUnpack(t *testing.T) {
	// From https://github.com/miekg/dns/issues/150#issuecomment-62296803
	// It should be an update message for the zone "example.",
	// deleting the A RRset "example." and then adding an A record at "example.".
	// class ANY, TYPE A
	buf := []byte{171, 68, 40, 0, 0, 1, 0, 0, 0, 2, 0, 0, 7, 101, 120, 97, 109, 112, 108, 101, 0, 0, 6, 0, 1, 192, 12, 0, 1, 0, 255, 0, 0, 0, 0, 0, 0, 192, 12, 0, 1, 0, 1, 0, 0, 0, 0, 0, 4, 127, 0, 0, 1}
	msg := new(Msg)
	err := msg.Unpack(buf)
	if err != nil {
		t.Errorf("failed to unpack: %v\n%s", err, msg.String())
	}
}

func TestDynamicUpdateZeroRdataUnpack(t *testing.T) {
	m := new(Msg)
	rr := &RR_Header{Name: ".", Rrtype: 0, Class: 1, Ttl: ^uint32(0), Rdlength: 0}
	m.Answer = []RR{rr, rr, rr, rr, rr}
	m.Ns = m.Answer
	for n, s := range TypeToString {
		rr.Rrtype = n
		bytes, err := m.Pack()
		if err != nil {
			t.Errorf("failed to pack %s: %v", s, err)
			continue
		}
		if err := new(Msg).Unpack(bytes); err != nil {
			t.Errorf("failed to unpack %s: %v", s, err)
		}
	}
}

func TestRemoveRRset(t *testing.T) {
	// Should add a zero data RR in Class ANY with a TTL of 0
	// for each set mentioned in the RRs provided to it.
	rr := testRR(". 100 IN A 127.0.0.1")
	m := new(Msg)
	m.Ns = []RR{&RR_Header{Name: ".", Rrtype: TypeA, Class: ClassANY, Ttl: 0, Rdlength: 0}}
	expectstr := m.String()
	expect, err := m.Pack()
	if err != nil {
		t.Fatalf("error packing expected msg: %v", err)
	}

	m.Ns = nil
	m.RemoveRRset([]RR{rr})
	actual, err := m.Pack()
	if err != nil {
		t.Fatalf("error packing actual msg: %v", err)
	}
	if !bytes.Equal(actual, expect) {
		tmp := new(Msg)
		if err := tmp.Unpack(actual); err != nil {
			t.Fatalf("error unpacking actual msg: %v\nexpected: %v\ngot: %v\n", err, expect, actual)
		}
		t.Errorf("expected msg:\n%s", expectstr)
		t.Errorf("actual msg:\n%v", tmp)
	}
}

func TestPreReqAndRemovals(t *testing.T) {
	// Build a list of multiple prereqs and then somes removes followed by an insert.
	// We should be able to add multiple prereqs and updates.
	m := new(Msg)
	m.SetUpdate("example.org.")
	m.Id = 1234

	// Use a full set of RRs each time, so we are sure the rdata is stripped.
	rrName1 := testRR("name_used. 3600 IN A 127.0.0.1")
	rrName2 := testRR("name_not_used. 3600 IN A 127.0.0.1")
	rrRemove1 := testRR("remove1. 3600 IN A 127.0.0.1")
	rrRemove2 := testRR("remove2. 3600 IN A 127.0.0.1")
	rrRemove3 := testRR("remove3. 3600 IN A 127.0.0.1")
	rrInsert := testRR("insert. 3600 IN A 127.0.0.1")
	rrRrset1 := testRR("rrset_used1. 3600 IN A 127.0.0.1")
	rrRrset2 := testRR("rrset_used2. 3600 IN A 127.0.0.1")
	rrRrset3 := testRR("rrset_not_used. 3600 IN A 127.0.0.1")

	// Handle the prereqs.
	m.NameUsed([]RR{rrName1})
	m.NameNotUsed([]RR{rrName2})
	m.RRsetUsed([]RR{rrRrset1})
	m.Used([]RR{rrRrset2})
	m.RRsetNotUsed([]RR{rrRrset3})

	// and now the updates.
	m.RemoveName([]RR{rrRemove1})
	m.RemoveRRset([]RR{rrRemove2})
	m.Remove([]RR{rrRemove3})
	m.Insert([]RR{rrInsert})

	// This test function isn't a Example function because we print these RR with tabs at the
	// end and the Example function trim these, thus they never match.
	// TODO(miek): don't print these tabs and make this into an Example function.
	expect := `;; opcode: UPDATE, status: NOERROR, id: 1234
;; flags:; QUERY: 1, ANSWER: 5, AUTHORITY: 4, ADDITIONAL: 0

;; QUESTION SECTION:
;example.org.	IN	 SOA

;; ANSWER SECTION:
name_used.	0	CLASS255	ANY	
name_not_used.	0	NONE	ANY	
rrset_used1.	0	CLASS255	A	
rrset_used2.	3600	IN	A	127.0.0.1
rrset_not_used.	0	NONE	A	

;; AUTHORITY SECTION:
remove1.	0	CLASS255	ANY	
remove2.	0	CLASS255	A	
remove3.	0	NONE	A	127.0.0.1
insert.	3600	IN	A	127.0.0.1
`

	if m.String() != expect {
		t.Errorf("expected msg:\n%s", expect)
		t.Errorf("actual msg:\n%v", m.String())
	}
}
