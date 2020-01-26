package serf

import (
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

func TestInternalQueryName(t *testing.T) {
	name := internalQueryName(conflictQuery)
	if name != "_serf_conflict" {
		t.Fatalf("bad: %v", name)
	}
}

func TestSerfQueries_Passthrough(t *testing.T) {
	serf := &Serf{}
	logger := log.New(os.Stderr, "", log.LstdFlags)
	outCh := make(chan Event, 4)
	shutdown := make(chan struct{})
	defer close(shutdown)
	eventCh, err := newSerfQueries(serf, logger, outCh, shutdown)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Push a user event
	eventCh <- UserEvent{LTime: 42, Name: "foo"}

	// Push a query
	eventCh <- &Query{LTime: 42, Name: "foo"}

	// Push a query
	eventCh <- MemberEvent{Type: EventMemberJoin}

	// Should get passed through
	for i := 0; i < 3; i++ {
		select {
		case <-outCh:
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("time out")
		}
	}
}

func TestSerfQueries_Ping(t *testing.T) {
	serf := &Serf{}
	logger := log.New(os.Stderr, "", log.LstdFlags)
	outCh := make(chan Event, 4)
	shutdown := make(chan struct{})
	defer close(shutdown)
	eventCh, err := newSerfQueries(serf, logger, outCh, shutdown)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Send a ping
	eventCh <- &Query{LTime: 42, Name: "_serf_ping"}

	// Should not get passed through
	select {
	case <-outCh:
		t.Fatalf("Should not passthrough query!")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSerfQueries_Conflict_SameName(t *testing.T) {
	serf := &Serf{config: &Config{NodeName: "foo"}}
	logger := log.New(os.Stderr, "", log.LstdFlags)
	outCh := make(chan Event, 4)
	shutdown := make(chan struct{})
	defer close(shutdown)
	eventCh, err := newSerfQueries(serf, logger, outCh, shutdown)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Query for our own name
	eventCh <- &Query{Name: "_serf_conflict", Payload: []byte("foo")}

	// Should not passthrough OR respond
	select {
	case <-outCh:
		t.Fatalf("Should not passthrough query!")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSerfQueries_estimateMaxKeysInListKeyResponseFactor(t *testing.T) {
	q := Query{id: 0, serf: &Serf{config: &Config{NodeName: "", QueryResponseSizeLimit: DefaultConfig().QueryResponseSizeLimit * 10}}}
	resp := nodeKeyResponse{Keys: []string{}}
	for i := 0; i <= q.serf.config.QueryResponseSizeLimit; i++ {
		resp.Keys = append(resp.Keys, "LeJcrRIZsZ9tPYJZW7Xllg==")
	}
	found := 0
	for i := len(resp.Keys); i >= 0; i-- {
		buf, err := encodeMessage(messageKeyResponseType, resp)
		if err != nil {
			t.Fatal(err)
		}
		qresp := q.createResponse(buf)
		raw, err := encodeMessage(messageQueryResponseType, qresp)
		if err != nil {
			t.Fatal(err)
		}
		if err = q.checkResponseSize(raw); err != nil {
			resp.Keys = resp.Keys[0:i]
			continue
		}
		found = i
		break
	}
	if found == 0 {
		t.Fatal("Didn't find anything!")
	}
	t.Logf("max keys in response with %d bytes: %d", q.serf.config.QueryResponseSizeLimit, len(resp.Keys))
	t.Logf("factor: %d", q.serf.config.QueryResponseSizeLimit/len(resp.Keys))
}

func TestSerfQueries_keyListResponseWithCorrectSize(t *testing.T) {
	s := serfQueries{logger: log.New(os.Stderr, "", log.LstdFlags)}
	q := Query{id: 0, serf: &Serf{config: &Config{NodeName: "", QueryResponseSizeLimit: 1024}}}
	cases := []struct {
		resp     nodeKeyResponse
		expected int
		hasMsg   bool
	}{
		{expected: 0, hasMsg: false, resp: nodeKeyResponse{}},
		{expected: 1, hasMsg: false, resp: nodeKeyResponse{Keys: []string{"LeJcrRIZsZ9tPYJZW7Xllg=="}}},
		// has 50 keys which makes the response bigger than 1024 bytes.
		{expected: 36, hasMsg: true, resp: nodeKeyResponse{Keys: []string{
			"LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==",
			"LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==",
			"LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==",
			"LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==",
			"LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==",
			"LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==",
			"LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==",
			"LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==",
			"LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==",
			"LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==", "LeJcrRIZsZ9tPYJZW7Xllg==",
		}}},
	}
	for _, c := range cases {
		r := c.resp
		_, _, err := s.keyListResponseWithCorrectSize(&q, &r)
		if err != nil {
			t.Error(err)
			continue
		}
		if len(r.Keys) != c.expected {
			t.Errorf("Expected %d vs %d", c.expected, len(r.Keys))
		}
		if c.hasMsg && !strings.Contains(r.Message, "truncated") {
			t.Error("truncation message should be set")
		}
	}
}
