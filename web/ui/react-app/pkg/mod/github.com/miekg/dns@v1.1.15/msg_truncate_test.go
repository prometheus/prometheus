package dns

import (
	"fmt"
	"testing"
)

func TestRequestTruncateAnswer(t *testing.T) {
	m := new(Msg)
	m.SetQuestion("large.example.com.", TypeSRV)

	reply := new(Msg)
	reply.SetReply(m)
	for i := 1; i < 200; i++ {
		reply.Answer = append(reply.Answer, testRR(
			fmt.Sprintf("large.example.com. 10 IN SRV 0 0 80 10-0-0-%d.default.pod.k8s.example.com.", i)))
	}

	reply.Truncate(MinMsgSize)
	if want, got := MinMsgSize, reply.Len(); want < got {
		t.Errorf("message length should be bellow %d bytes, got %d bytes", want, got)
	}
	if !reply.Truncated {
		t.Errorf("truncated bit should be set")
	}
}

func TestRequestTruncateExtra(t *testing.T) {
	m := new(Msg)
	m.SetQuestion("large.example.com.", TypeSRV)

	reply := new(Msg)
	reply.SetReply(m)
	for i := 1; i < 200; i++ {
		reply.Extra = append(reply.Extra, testRR(
			fmt.Sprintf("large.example.com. 10 IN SRV 0 0 80 10-0-0-%d.default.pod.k8s.example.com.", i)))
	}

	reply.Truncate(MinMsgSize)
	if want, got := MinMsgSize, reply.Len(); want < got {
		t.Errorf("message length should be bellow %d bytes, got %d bytes", want, got)
	}
	if !reply.Truncated {
		t.Errorf("truncated bit should be set")
	}
}

func TestRequestTruncateExtraEdns0(t *testing.T) {
	const size = 4096

	m := new(Msg)
	m.SetQuestion("large.example.com.", TypeSRV)
	m.SetEdns0(size, true)

	reply := new(Msg)
	reply.SetReply(m)
	for i := 1; i < 200; i++ {
		reply.Extra = append(reply.Extra, testRR(
			fmt.Sprintf("large.example.com. 10 IN SRV 0 0 80 10-0-0-%d.default.pod.k8s.example.com.", i)))
	}
	reply.SetEdns0(size, true)

	reply.Truncate(size)
	if want, got := size, reply.Len(); want < got {
		t.Errorf("message length should be bellow %d bytes, got %d bytes", want, got)
	}
	if !reply.Truncated {
		t.Errorf("truncated bit should be set")
	}
	opt := reply.Extra[len(reply.Extra)-1]
	if opt.Header().Rrtype != TypeOPT {
		t.Errorf("expected last RR to be OPT")
	}
}

func TestRequestTruncateExtraRegression(t *testing.T) {
	const size = 2048

	m := new(Msg)
	m.SetQuestion("large.example.com.", TypeSRV)
	m.SetEdns0(size, true)

	reply := new(Msg)
	reply.SetReply(m)
	for i := 1; i < 33; i++ {
		reply.Answer = append(reply.Answer, testRR(
			fmt.Sprintf("large.example.com. 10 IN SRV 0 0 80 10-0-0-%d.default.pod.k8s.example.com.", i)))
	}
	for i := 1; i < 33; i++ {
		reply.Extra = append(reply.Extra, testRR(
			fmt.Sprintf("10-0-0-%d.default.pod.k8s.example.com. 10 IN A 10.0.0.%d", i, i)))
	}
	reply.SetEdns0(size, true)

	reply.Truncate(size)
	if want, got := size, reply.Len(); want < got {
		t.Errorf("message length should be bellow %d bytes, got %d bytes", want, got)
	}
	if !reply.Truncated {
		t.Errorf("truncated bit should be set")
	}
	opt := reply.Extra[len(reply.Extra)-1]
	if opt.Header().Rrtype != TypeOPT {
		t.Errorf("expected last RR to be OPT")
	}
}

func TestTruncation(t *testing.T) {
	reply := new(Msg)

	for i := 0; i < 61; i++ {
		reply.Answer = append(reply.Answer, testRR(fmt.Sprintf("http.service.tcp.srv.k8s.example.org. 5 IN SRV 0 0 80 10-144-230-%d.default.pod.k8s.example.org.", i)))
	}

	for i := 0; i < 5; i++ {
		reply.Extra = append(reply.Extra, testRR(fmt.Sprintf("ip-10-10-52-5%d.subdomain.example.org. 5 IN A 10.10.52.5%d", i, i)))
	}

	for i := 0; i < 5; i++ {
		reply.Ns = append(reply.Ns, testRR(fmt.Sprintf("srv.subdomain.example.org. 5 IN NS ip-10-10-33-6%d.subdomain.example.org.", i)))
	}

	for bufsize := 1024; bufsize <= 4096; bufsize += 12 {
		m := new(Msg)
		m.SetQuestion("http.service.tcp.srv.k8s.example.org.", TypeSRV)
		m.SetEdns0(uint16(bufsize), true)

		copy := reply.Copy()
		copy.SetReply(m)

		copy.Truncate(bufsize)
		if want, got := bufsize, copy.Len(); want < got {
			t.Errorf("message length should be bellow %d bytes, got %d bytes", want, got)
		}
	}
}

func TestRequestTruncateAnswerExact(t *testing.T) {
	const size = 867 // Bit fiddly, but this hits the rl == size break clause in Truncate, 52 RRs should remain.

	m := new(Msg)
	m.SetQuestion("large.example.com.", TypeSRV)
	m.SetEdns0(size, false)

	reply := new(Msg)
	reply.SetReply(m)
	for i := 1; i < 200; i++ {
		reply.Answer = append(reply.Answer, testRR(fmt.Sprintf("large.example.com. 10 IN A 127.0.0.%d", i)))
	}

	reply.Truncate(size)
	if want, got := size, reply.Len(); want < got {
		t.Errorf("message length should be bellow %d bytes, got %d bytes", want, got)
	}
	if expected := 52; len(reply.Answer) != expected {
		t.Errorf("wrong number of answers; expected %d, got %d", expected, len(reply.Answer))
	}
}

func BenchmarkMsgTruncate(b *testing.B) {
	const size = 2048

	m := new(Msg)
	m.SetQuestion("example.com.", TypeA)
	m.SetEdns0(size, true)

	reply := new(Msg)
	reply.SetReply(m)
	for i := 1; i < 33; i++ {
		reply.Answer = append(reply.Answer, testRR(
			fmt.Sprintf("large.example.com. 10 IN SRV 0 0 80 10-0-0-%d.default.pod.k8s.example.com.", i)))
	}
	for i := 1; i < 33; i++ {
		reply.Extra = append(reply.Extra, testRR(
			fmt.Sprintf("10-0-0-%d.default.pod.k8s.example.com. 10 IN A 10.0.0.%d", i, i)))
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		copy := reply.Copy()
		b.StartTimer()

		copy.Truncate(size)
	}
}
