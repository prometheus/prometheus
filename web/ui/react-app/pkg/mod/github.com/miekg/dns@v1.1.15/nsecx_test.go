package dns

import (
	"strconv"
	"testing"
)

func TestPackNsec3(t *testing.T) {
	nsec3 := HashName("dnsex.nl.", SHA1, 0, "DEAD")
	if nsec3 != "ROCCJAE8BJJU7HN6T7NG3TNM8ACRS87J" {
		t.Error(nsec3)
	}

	nsec3 = HashName("a.b.c.example.org.", SHA1, 2, "DEAD")
	if nsec3 != "6LQ07OAHBTOOEU2R9ANI2AT70K5O0RCG" {
		t.Error(nsec3)
	}
}

func TestNsec3(t *testing.T) {
	nsec3 := testRR("sk4e8fj94u78smusb40o1n0oltbblu2r.nl. IN NSEC3 1 1 5 F10E9F7EA83FC8F3 SK4F38CQ0ATIEI8MH3RGD0P5I4II6QAN NS SOA TXT RRSIG DNSKEY NSEC3PARAM")
	if !nsec3.(*NSEC3).Match("nl.") { // name hash = sk4e8fj94u78smusb40o1n0oltbblu2r
		t.Fatal("sk4e8fj94u78smusb40o1n0oltbblu2r.nl. should match sk4e8fj94u78smusb40o1n0oltbblu2r.nl.")
	}
	if !nsec3.(*NSEC3).Match("NL.") { // name hash = sk4e8fj94u78smusb40o1n0oltbblu2r
		t.Fatal("sk4e8fj94u78smusb40o1n0oltbblu2r.NL. should match sk4e8fj94u78smusb40o1n0oltbblu2r.nl.")
	}
	if nsec3.(*NSEC3).Match("com.") { //
		t.Fatal("com. is not in the zone nl.")
	}
	if nsec3.(*NSEC3).Match("test.nl.") { // name hash = gd0ptr5bnfpimpu2d3v6gd4n0bai7s0q
		t.Fatal("gd0ptr5bnfpimpu2d3v6gd4n0bai7s0q.nl. should not match sk4e8fj94u78smusb40o1n0oltbblu2r.nl.")
	}
	nsec3 = testRR("nl. IN NSEC3 1 1 5 F10E9F7EA83FC8F3 SK4F38CQ0ATIEI8MH3RGD0P5I4II6QAN NS SOA TXT RRSIG DNSKEY NSEC3PARAM")
	if nsec3.(*NSEC3).Match("nl.") {
		t.Fatal("sk4e8fj94u78smusb40o1n0oltbblu2r.nl. should not match a record without a owner hash")
	}

	for _, tc := range []struct {
		rr     *NSEC3
		name   string
		covers bool
	}{
		// positive tests
		{ // name hash between owner hash and next hash
			rr: &NSEC3{
				Hdr:        RR_Header{Name: "2N1TB3VAIRUOBL6RKDVII42N9TFMIALP.com."},
				Hash:       1,
				Flags:      1,
				Iterations: 5,
				Salt:       "F10E9F7EA83FC8F3",
				NextDomain: "PT3RON8N7PM3A0OE989IB84OOSADP7O8",
			},
			name:   "bsd.com.",
			covers: true,
		},
		{ // end of zone, name hash is after owner hash
			rr: &NSEC3{
				Hdr:        RR_Header{Name: "3v62ulr0nre83v0rja2vjgtlif9v6rab.com."},
				Hash:       1,
				Flags:      1,
				Iterations: 5,
				Salt:       "F10E9F7EA83FC8F3",
				NextDomain: "2N1TB3VAIRUOBL6RKDVII42N9TFMIALP",
			},
			name:   "csd.com.",
			covers: true,
		},
		{ // end of zone, name hash is before beginning of zone
			rr: &NSEC3{
				Hdr:        RR_Header{Name: "PT3RON8N7PM3A0OE989IB84OOSADP7O8.com."},
				Hash:       1,
				Flags:      1,
				Iterations: 5,
				Salt:       "F10E9F7EA83FC8F3",
				NextDomain: "3V62ULR0NRE83V0RJA2VJGTLIF9V6RAB",
			},
			name:   "asd.com.",
			covers: true,
		},
		// negative tests
		{ // too short owner name
			rr: &NSEC3{
				Hdr:        RR_Header{Name: "nl."},
				Hash:       1,
				Flags:      1,
				Iterations: 5,
				Salt:       "F10E9F7EA83FC8F3",
				NextDomain: "39P99DCGG0MDLARTCRMCF6OFLLUL7PR6",
			},
			name:   "asd.com.",
			covers: false,
		},
		{ // outside of zone
			rr: &NSEC3{
				Hdr:        RR_Header{Name: "39p91242oslggest5e6a7cci4iaeqvnk.nl."},
				Hash:       1,
				Flags:      1,
				Iterations: 5,
				Salt:       "F10E9F7EA83FC8F3",
				NextDomain: "39P99DCGG0MDLARTCRMCF6OFLLUL7PR6",
			},
			name:   "asd.com.",
			covers: false,
		},
		{ // empty interval
			rr: &NSEC3{
				Hdr:        RR_Header{Name: "2n1tb3vairuobl6rkdvii42n9tfmialp.com."},
				Hash:       1,
				Flags:      1,
				Iterations: 5,
				Salt:       "F10E9F7EA83FC8F3",
				NextDomain: "2N1TB3VAIRUOBL6RKDVII42N9TFMIALP",
			},
			name:   "asd.com.",
			covers: false,
		},
		{ // empty interval wildcard
			rr: &NSEC3{
				Hdr:        RR_Header{Name: "2n1tb3vairuobl6rkdvii42n9tfmialp.com."},
				Hash:       1,
				Flags:      1,
				Iterations: 5,
				Salt:       "F10E9F7EA83FC8F3",
				NextDomain: "2N1TB3VAIRUOBL6RKDVII42N9TFMIALP",
			},
			name:   "*.asd.com.",
			covers: true,
		},
		{ // name hash is before owner hash, not covered
			rr: &NSEC3{
				Hdr:        RR_Header{Name: "3V62ULR0NRE83V0RJA2VJGTLIF9V6RAB.com."},
				Hash:       1,
				Flags:      1,
				Iterations: 5,
				Salt:       "F10E9F7EA83FC8F3",
				NextDomain: "PT3RON8N7PM3A0OE989IB84OOSADP7O8",
			},
			name:   "asd.com.",
			covers: false,
		},
	} {
		covers := tc.rr.Cover(tc.name)
		if tc.covers != covers {
			t.Fatalf("cover failed for %s: expected %t, got %t [record: %s]", tc.name, tc.covers, covers, tc.rr)
		}
	}
}

func TestNsec3EmptySalt(t *testing.T) {
	rr, _ := NewRR("CK0POJMG874LJREF7EFN8430QVIT8BSM.com. 86400 IN NSEC3 1 1 0 - CK0Q1GIN43N1ARRC9OSM6QPQR81H5M9A  NS SOA RRSIG DNSKEY NSEC3PARAM")

	if !rr.(*NSEC3).Match("com.") {
		t.Fatalf("expected record to match com. label")
	}
}

func BenchmarkHashName(b *testing.B) {
	for _, iter := range []uint16{
		150, 2500, 5000, 10000, ^uint16(0),
	} {
		b.Run(strconv.Itoa(int(iter)), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				if HashName("some.example.org.", SHA1, iter, "deadbeef") == "" {
					b.Fatalf("HashName failed")
				}
			}
		})
	}
}
