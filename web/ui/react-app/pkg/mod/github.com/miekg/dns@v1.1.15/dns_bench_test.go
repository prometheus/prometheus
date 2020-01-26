package dns

import (
	"fmt"
	"net"
	"testing"
)

func BenchmarkMsgLength(b *testing.B) {
	b.StopTimer()
	makeMsg := func(question string, ans, ns, e []RR) *Msg {
		msg := new(Msg)
		msg.SetQuestion(Fqdn(question), TypeANY)
		msg.Answer = append(msg.Answer, ans...)
		msg.Ns = append(msg.Ns, ns...)
		msg.Extra = append(msg.Extra, e...)
		msg.Compress = true
		return msg
	}
	name1 := "12345678901234567890123456789012345.12345678.123."
	rrMx := testRR(name1 + " 3600 IN MX 10 " + name1)
	msg := makeMsg(name1, []RR{rrMx, rrMx}, nil, nil)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		msg.Len()
	}
}

func BenchmarkMsgLengthNoCompression(b *testing.B) {
	b.StopTimer()
	makeMsg := func(question string, ans, ns, e []RR) *Msg {
		msg := new(Msg)
		msg.SetQuestion(Fqdn(question), TypeANY)
		msg.Answer = append(msg.Answer, ans...)
		msg.Ns = append(msg.Ns, ns...)
		msg.Extra = append(msg.Extra, e...)
		return msg
	}
	name1 := "12345678901234567890123456789012345.12345678.123."
	rrMx := testRR(name1 + " 3600 IN MX 10 " + name1)
	msg := makeMsg(name1, []RR{rrMx, rrMx}, nil, nil)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		msg.Len()
	}
}

func BenchmarkMsgLengthPack(b *testing.B) {
	makeMsg := func(question string, ans, ns, e []RR) *Msg {
		msg := new(Msg)
		msg.SetQuestion(Fqdn(question), TypeANY)
		msg.Answer = append(msg.Answer, ans...)
		msg.Ns = append(msg.Ns, ns...)
		msg.Extra = append(msg.Extra, e...)
		msg.Compress = true
		return msg
	}
	name1 := "12345678901234567890123456789012345.12345678.123."
	rrMx := testRR(name1 + " 3600 IN MX 10 " + name1)
	msg := makeMsg(name1, []RR{rrMx, rrMx}, nil, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = msg.Pack()
	}
}

func BenchmarkMsgLengthMassive(b *testing.B) {
	makeMsg := func(question string, ans, ns, e []RR) *Msg {
		msg := new(Msg)
		msg.SetQuestion(Fqdn(question), TypeANY)
		msg.Answer = append(msg.Answer, ans...)
		msg.Ns = append(msg.Ns, ns...)
		msg.Extra = append(msg.Extra, e...)
		msg.Compress = true
		return msg
	}
	const name1 = "12345678901234567890123456789012345.12345678.123."
	rrMx := testRR(name1 + " 3600 IN MX 10 " + name1)
	answer := []RR{rrMx, rrMx}
	for i := 0; i < 128; i++ {
		rrA := testRR(fmt.Sprintf("example%03d.something%03delse.org. 2311 IN A 127.0.0.1", i/32, i%32))
		answer = append(answer, rrA)
	}
	answer = append(answer, rrMx, rrMx)
	msg := makeMsg(name1, answer, nil, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg.Len()
	}
}

func BenchmarkMsgLengthOnlyQuestion(b *testing.B) {
	msg := new(Msg)
	msg.SetQuestion(Fqdn("12345678901234567890123456789012345.12345678.123."), TypeANY)
	msg.Compress = true
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg.Len()
	}
}

func BenchmarkMsgLengthEscapedName(b *testing.B) {
	msg := new(Msg)
	msg.SetQuestion(`\1\2\3\4\5\6\7\8\9\0\1\2\3\4\5\6\7\8\9\0\1\2\3\4\5\6\7\8\9\0\1\2\3\4\5.\1\2\3\4\5\6\7\8.\1\2\3.`, TypeANY)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg.Len()
	}
}

func BenchmarkPackDomainName(b *testing.B) {
	name1 := "12345678901234567890123456789012345.12345678.123."
	buf := make([]byte, len(name1)+1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = PackDomainName(name1, buf, 0, nil, false)
	}
}

func BenchmarkUnpackDomainName(b *testing.B) {
	name1 := "12345678901234567890123456789012345.12345678.123."
	buf := make([]byte, len(name1)+1)
	_, _ = PackDomainName(name1, buf, 0, nil, false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = UnpackDomainName(buf, 0)
	}
}

func BenchmarkUnpackDomainNameUnprintable(b *testing.B) {
	name1 := "\x02\x02\x02\x025\x02\x02\x02\x02.12345678.123."
	buf := make([]byte, len(name1)+1)
	_, _ = PackDomainName(name1, buf, 0, nil, false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = UnpackDomainName(buf, 0)
	}
}

func BenchmarkUnpackDomainNameLongest(b *testing.B) {
	buf := make([]byte, len(longestDomain)+1)
	n, err := PackDomainName(longestDomain, buf, 0, nil, false)
	if err != nil {
		b.Fatal(err)
	}
	if n != maxDomainNameWireOctets {
		b.Fatalf("name wrong size in wire format, expected %d, got %d", maxDomainNameWireOctets, n)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = UnpackDomainName(buf, 0)
	}
}

func BenchmarkUnpackDomainNameLongestUnprintable(b *testing.B) {
	buf := make([]byte, len(longestUnprintableDomain)+1)
	n, err := PackDomainName(longestUnprintableDomain, buf, 0, nil, false)
	if err != nil {
		b.Fatal(err)
	}
	if n != maxDomainNameWireOctets {
		b.Fatalf("name wrong size in wire format, expected %d, got %d", maxDomainNameWireOctets, n)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = UnpackDomainName(buf, 0)
	}
}

func BenchmarkCopy(b *testing.B) {
	b.ReportAllocs()
	m := new(Msg)
	m.SetQuestion("miek.nl.", TypeA)
	rr := testRR("miek.nl. 2311 IN A 127.0.0.1")
	m.Answer = []RR{rr}
	rr = testRR("miek.nl. 2311 IN NS 127.0.0.1")
	m.Ns = []RR{rr}
	rr = testRR("miek.nl. 2311 IN A 127.0.0.1")
	m.Extra = []RR{rr}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Copy()
	}
}

func BenchmarkPackA(b *testing.B) {
	a := &A{Hdr: RR_Header{Name: ".", Rrtype: TypeA, Class: ClassANY}, A: net.IPv4(127, 0, 0, 1)}

	buf := make([]byte, Len(a))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = PackRR(a, buf, 0, nil, false)
	}
}

func BenchmarkUnpackA(b *testing.B) {
	a := &A{Hdr: RR_Header{Name: ".", Rrtype: TypeA, Class: ClassANY}, A: net.IPv4(127, 0, 0, 1)}

	buf := make([]byte, Len(a))
	PackRR(a, buf, 0, nil, false)
	a = nil
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = UnpackRR(buf, 0)
	}
}

func BenchmarkPackMX(b *testing.B) {
	m := &MX{Hdr: RR_Header{Name: ".", Rrtype: TypeA, Class: ClassANY}, Mx: "mx.miek.nl."}

	buf := make([]byte, Len(m))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = PackRR(m, buf, 0, nil, false)
	}
}

func BenchmarkUnpackMX(b *testing.B) {
	m := &MX{Hdr: RR_Header{Name: ".", Rrtype: TypeA, Class: ClassANY}, Mx: "mx.miek.nl."}

	buf := make([]byte, Len(m))
	PackRR(m, buf, 0, nil, false)
	m = nil
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = UnpackRR(buf, 0)
	}
}

func BenchmarkPackAAAAA(b *testing.B) {
	aaaa := testRR(". IN AAAA ::1")

	buf := make([]byte, Len(aaaa))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = PackRR(aaaa, buf, 0, nil, false)
	}
}

func BenchmarkUnpackAAAA(b *testing.B) {
	aaaa := testRR(". IN AAAA ::1")

	buf := make([]byte, Len(aaaa))
	PackRR(aaaa, buf, 0, nil, false)
	aaaa = nil
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = UnpackRR(buf, 0)
	}
}

func BenchmarkPackMsg(b *testing.B) {
	makeMsg := func(question string, ans, ns, e []RR) *Msg {
		msg := new(Msg)
		msg.SetQuestion(Fqdn(question), TypeANY)
		msg.Answer = append(msg.Answer, ans...)
		msg.Ns = append(msg.Ns, ns...)
		msg.Extra = append(msg.Extra, e...)
		msg.Compress = true
		return msg
	}
	name1 := "12345678901234567890123456789012345.12345678.123."
	rrMx := testRR(name1 + " 3600 IN MX 10 " + name1)
	msg := makeMsg(name1, []RR{rrMx, rrMx}, nil, nil)
	buf := make([]byte, 512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = msg.PackBuffer(buf)
	}
}

func BenchmarkPackMsgMassive(b *testing.B) {
	makeMsg := func(question string, ans, ns, e []RR) *Msg {
		msg := new(Msg)
		msg.SetQuestion(Fqdn(question), TypeANY)
		msg.Answer = append(msg.Answer, ans...)
		msg.Ns = append(msg.Ns, ns...)
		msg.Extra = append(msg.Extra, e...)
		msg.Compress = true
		return msg
	}
	const name1 = "12345678901234567890123456789012345.12345678.123."
	rrMx := testRR(name1 + " 3600 IN MX 10 " + name1)
	answer := []RR{rrMx, rrMx}
	for i := 0; i < 128; i++ {
		rrA := testRR(fmt.Sprintf("example%03d.something%03delse.org. 2311 IN A 127.0.0.1", i/32, i%32))
		answer = append(answer, rrA)
	}
	answer = append(answer, rrMx, rrMx)
	msg := makeMsg(name1, answer, nil, nil)
	buf := make([]byte, 512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = msg.PackBuffer(buf)
	}
}

func BenchmarkPackMsgOnlyQuestion(b *testing.B) {
	msg := new(Msg)
	msg.SetQuestion(Fqdn("12345678901234567890123456789012345.12345678.123."), TypeANY)
	msg.Compress = true
	buf := make([]byte, 512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = msg.PackBuffer(buf)
	}
}

func BenchmarkUnpackMsg(b *testing.B) {
	makeMsg := func(question string, ans, ns, e []RR) *Msg {
		msg := new(Msg)
		msg.SetQuestion(Fqdn(question), TypeANY)
		msg.Answer = append(msg.Answer, ans...)
		msg.Ns = append(msg.Ns, ns...)
		msg.Extra = append(msg.Extra, e...)
		msg.Compress = true
		return msg
	}
	name1 := "12345678901234567890123456789012345.12345678.123."
	rrMx := testRR(name1 + " 3600 IN MX 10 " + name1)
	msg := makeMsg(name1, []RR{rrMx, rrMx}, nil, nil)
	msgBuf, _ := msg.Pack()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = msg.Unpack(msgBuf)
	}
}

func BenchmarkIdGeneration(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = id()
	}
}

func BenchmarkReverseAddr(b *testing.B) {
	b.Run("IP4", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			addr, err := ReverseAddr("192.0.2.1")
			if err != nil {
				b.Fatal(err)
			}
			if expect := "1.2.0.192.in-addr.arpa."; addr != expect {
				b.Fatalf("invalid reverse address, expected %q, got %q", expect, addr)
			}
		}
	})

	b.Run("IP6", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			addr, err := ReverseAddr("2001:db8::68")
			if err != nil {
				b.Fatal(err)
			}
			if expect := "8.6.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa."; addr != expect {
				b.Fatalf("invalid reverse address, expected %q, got %q", expect, addr)
			}
		}
	})
}
