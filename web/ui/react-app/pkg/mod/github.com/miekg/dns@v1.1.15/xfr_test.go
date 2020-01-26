package dns

import (
	"net"
	"sync"
	"testing"
	"time"
)

var (
	tsigSecret  = map[string]string{"axfr.": "so6ZGir4GPAqINNh9U5c3A=="}
	xfrSoa      = testRR(`miek.nl.	0	IN	SOA	linode.atoom.net. miek.miek.nl. 2009032802 21600 7200 604800 3600`)
	xfrA        = testRR(`x.miek.nl.	1792	IN	A	10.0.0.1`)
	xfrMX       = testRR(`miek.nl.	1800	IN	MX	1	x.miek.nl.`)
	xfrTestData = []RR{xfrSoa, xfrA, xfrMX, xfrSoa}
)

func InvalidXfrServer(w ResponseWriter, req *Msg) {
	ch := make(chan *Envelope)
	tr := new(Transfer)

	go tr.Out(w, req, ch)
	ch <- &Envelope{RR: []RR{}}
	close(ch)
	w.Hijack()
}

func SingleEnvelopeXfrServer(w ResponseWriter, req *Msg) {
	ch := make(chan *Envelope)
	tr := new(Transfer)

	go tr.Out(w, req, ch)
	ch <- &Envelope{RR: xfrTestData}
	close(ch)
	w.Hijack()
}

func MultipleEnvelopeXfrServer(w ResponseWriter, req *Msg) {
	ch := make(chan *Envelope)
	tr := new(Transfer)

	go tr.Out(w, req, ch)

	for _, rr := range xfrTestData {
		ch <- &Envelope{RR: []RR{rr}}
	}
	close(ch)
	w.Hijack()
}

func TestInvalidXfr(t *testing.T) {
	HandleFunc("miek.nl.", InvalidXfrServer)
	defer HandleRemove("miek.nl.")

	s, addrstr, err := RunLocalTCPServer(":0")
	if err != nil {
		t.Fatalf("unable to run test server: %s", err)
	}
	defer s.Shutdown()

	tr := new(Transfer)
	m := new(Msg)
	m.SetAxfr("miek.nl.")

	c, err := tr.In(m, addrstr)
	if err != nil {
		t.Fatal("failed to zone transfer in", err)
	}

	for msg := range c {
		if msg.Error == nil {
			t.Fatal("failed to catch 'no SOA' error")
		}
	}
}

func TestSingleEnvelopeXfr(t *testing.T) {
	HandleFunc("miek.nl.", SingleEnvelopeXfrServer)
	defer HandleRemove("miek.nl.")

	s, addrstr, err := RunLocalTCPServerWithTsig(":0", tsigSecret)
	if err != nil {
		t.Fatalf("unable to run test server: %s", err)
	}
	defer s.Shutdown()

	axfrTestingSuite(addrstr)
}

func TestMultiEnvelopeXfr(t *testing.T) {
	HandleFunc("miek.nl.", MultipleEnvelopeXfrServer)
	defer HandleRemove("miek.nl.")

	s, addrstr, err := RunLocalTCPServerWithTsig(":0", tsigSecret)
	if err != nil {
		t.Fatalf("unable to run test server: %s", err)
	}
	defer s.Shutdown()

	axfrTestingSuite(addrstr)
}

func RunLocalTCPServerWithTsig(laddr string, tsig map[string]string) (*Server, string, error) {
	server, l, _, err := RunLocalTCPServerWithFinChanWithTsig(laddr, tsig)

	return server, l, err
}

func RunLocalTCPServerWithFinChanWithTsig(laddr string, tsig map[string]string) (*Server, string, chan error, error) {
	l, err := net.Listen("tcp", laddr)
	if err != nil {
		return nil, "", nil, err
	}

	server := &Server{Listener: l, ReadTimeout: time.Hour, WriteTimeout: time.Hour, TsigSecret: tsig}

	waitLock := sync.Mutex{}
	waitLock.Lock()
	server.NotifyStartedFunc = waitLock.Unlock

	// See the comment in RunLocalUDPServerWithFinChan as to
	// why fin must be buffered.
	fin := make(chan error, 1)

	go func() {
		fin <- server.ActivateAndServe()
		l.Close()
	}()

	waitLock.Lock()
	return server, l.Addr().String(), fin, nil
}

func axfrTestingSuite(addrstr string) func(*testing.T) {
	return func(t *testing.T) {
		tr := new(Transfer)
		m := new(Msg)
		m.SetAxfr("miek.nl.")

		c, err := tr.In(m, addrstr)
		if err != nil {
			t.Fatal("failed to zone transfer in", err)
		}

		var records []RR
		for msg := range c {
			if msg.Error != nil {
				t.Fatal(msg.Error)
			}
			for _, rr := range msg.RR {
				records = append(records, rr)
			}
		}

		if len(records) != len(xfrTestData) {
			t.Fatalf("bad axfr: expected %v, got %v", records, xfrTestData)
		}

		for i := range records {
			if !IsDuplicate(records[i], xfrTestData[i]) {
				t.Fatalf("bad axfr: expected %v, got %v", records, xfrTestData)
			}
		}
	}
}
