package dns

import (
	"testing"
)

func TestAcceptNotify(t *testing.T) {
	HandleFunc("example.org.", handleNotify)
	s, addrstr, err := RunLocalUDPServer(":0")
	if err != nil {
		t.Fatalf("unable to run test server: %v", err)
	}
	defer s.Shutdown()

	m := new(Msg)
	m.SetNotify("example.org.")
	// Set a SOA hint in the answer section, this is allowed according to RFC 1996.
	soa, _ := NewRR("example.org. IN SOA sns.dns.icann.org. noc.dns.icann.org. 2018112827 7200 3600 1209600 3600")
	m.Answer = []RR{soa}

	c := new(Client)
	resp, _, err := c.Exchange(m, addrstr)
	if err != nil {
		t.Errorf("failed to exchange: %v", err)
	}
	if resp.Rcode != RcodeSuccess {
		t.Errorf("expected %s, got %s", RcodeToString[RcodeSuccess], RcodeToString[resp.Rcode])
	}
}

func handleNotify(w ResponseWriter, req *Msg) {
	m := new(Msg)
	m.SetReply(req)
	w.WriteMsg(m)
}
