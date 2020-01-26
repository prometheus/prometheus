package agent

import (
	"bytes"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/serf/serf"
)

type MockStreamClient struct {
	headers []*responseHeader
	objs    []interface{}
	err     error
}

func (m *MockStreamClient) Send(h *responseHeader, o interface{}) error {
	m.headers = append(m.headers, h)
	m.objs = append(m.objs, o)
	return m.err
}

func (m *MockStreamClient) RegisterQuery(q *serf.Query) uint64 {
	return 42
}

func TestIPCEventStream(t *testing.T) {
	sc := &MockStreamClient{}
	filters := ParseEventFilter("user:foobar,member-join,query:deploy")
	es := newEventStream(sc, filters, 42, log.New(os.Stderr, "", log.LstdFlags))
	defer es.Stop()

	es.HandleEvent(serf.UserEvent{
		LTime:    123,
		Name:     "foobar",
		Payload:  []byte("test"),
		Coalesce: true,
	})
	es.HandleEvent(serf.UserEvent{
		LTime:    124,
		Name:     "ignore",
		Payload:  []byte("test"),
		Coalesce: true,
	})
	es.HandleEvent(serf.MemberEvent{
		Type: serf.EventMemberJoin,
		Members: []serf.Member{
			serf.Member{
				Name:        "TestNode",
				Addr:        net.IP([]byte{127, 0, 0, 1}),
				Port:        12345,
				Tags:        map[string]string{"role": "node"},
				Status:      serf.StatusAlive,
				ProtocolMin: 0,
				ProtocolMax: 0,
				ProtocolCur: 0,
				DelegateMin: 0,
				DelegateMax: 0,
				DelegateCur: 0,
			},
		},
	})
	es.HandleEvent(&serf.Query{
		LTime:   125,
		Name:    "deploy",
		Payload: []byte("test"),
	})

	time.Sleep(5 * time.Millisecond)

	if len(sc.headers) != 3 {
		t.Fatalf("expected 2 messages!")
	}
	for _, h := range sc.headers {
		if h.Seq != 42 {
			t.Fatalf("bad seq")
		}
		if h.Error != "" {
			t.Fatalf("bad err")
		}
	}

	obj1 := sc.objs[0].(*userEventRecord)
	if obj1.Event != "user" {
		t.Fatalf("bad event: %#v", obj1)
	}
	if obj1.LTime != 123 {
		t.Fatalf("bad event: %#v", obj1)
	}
	if obj1.Name != "foobar" {
		t.Fatalf("bad event: %#v", obj1)
	}
	if bytes.Compare(obj1.Payload, []byte("test")) != 0 {
		t.Fatalf("bad event: %#v", obj1)
	}
	if !obj1.Coalesce {
		t.Fatalf("bad event: %#v", obj1)
	}

	obj2 := sc.objs[1].(*memberEventRecord)
	if obj2.Event != "member-join" {
		t.Fatalf("bad event: %#v", obj2)
	}
	mem1 := obj2.Members[0]
	if mem1.Name != "TestNode" {
		t.Fatalf("bad member: %#v", mem1)
	}
	if bytes.Compare(mem1.Addr, []byte{127, 0, 0, 1}) != 0 {
		t.Fatalf("bad member: %#v", mem1)
	}
	if mem1.Port != 12345 {
		t.Fatalf("bad member: %#v", mem1)
	}
	if mem1.Status != "alive" {
		t.Fatalf("bad member: %#v", mem1)
	}
	if mem1.ProtocolMin != 0 {
		t.Fatalf("bad member: %#v", mem1)
	}
	if mem1.ProtocolMax != 0 {
		t.Fatalf("bad member: %#v", mem1)
	}
	if mem1.ProtocolCur != 0 {
		t.Fatalf("bad member: %#v", mem1)
	}
	if mem1.DelegateMin != 0 {
		t.Fatalf("bad member: %#v", mem1)
	}
	if mem1.DelegateMax != 0 {
		t.Fatalf("bad member: %#v", mem1)
	}
	if mem1.DelegateCur != 0 {
		t.Fatalf("bad member: %#v", mem1)
	}

	obj3 := sc.objs[2].(*queryEventRecord)
	if obj3.Event != "query" {
		t.Fatalf("bad query: %#v", obj3)
	}
	if obj3.ID != 42 {
		t.Fatalf("bad query: %#v", obj3)
	}
	if obj3.LTime != 125 {
		t.Fatalf("bad query: %#v", obj3)
	}
	if obj3.Name != "deploy" {
		t.Fatalf("bad query: %#v", obj3)
	}
	if bytes.Compare(obj3.Payload, []byte("test")) != 0 {
		t.Fatalf("bad query: %#v", obj3)
	}

}
