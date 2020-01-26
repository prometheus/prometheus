package agent

import (
	"fmt"
	"log"

	"github.com/hashicorp/serf/serf"
)

type streamClient interface {
	Send(*responseHeader, interface{}) error
	RegisterQuery(*serf.Query) uint64
}

// eventStream is used to stream events to a client over IPC
type eventStream struct {
	client  streamClient
	eventCh chan serf.Event
	filters []EventFilter
	logger  *log.Logger
	seq     uint64
}

func newEventStream(client streamClient, filters []EventFilter, seq uint64, logger *log.Logger) *eventStream {
	es := &eventStream{
		client:  client,
		eventCh: make(chan serf.Event, 512),
		filters: filters,
		logger:  logger,
		seq:     seq,
	}
	go es.stream()
	return es
}

func (es *eventStream) HandleEvent(e serf.Event) {
	// Check the event
	for _, f := range es.filters {
		if f.Invoke(e) {
			goto HANDLE
		}
	}
	return

	// Do a non-blocking send
HANDLE:
	select {
	case es.eventCh <- e:
	default:
		es.logger.Printf("[WARN] agent.ipc: Dropping event to %v", es.client)
	}
}

func (es *eventStream) Stop() {
	close(es.eventCh)
}

func (es *eventStream) stream() {
	var err error
	for event := range es.eventCh {
		switch e := event.(type) {
		case serf.MemberEvent:
			err = es.sendMemberEvent(e)
		case serf.UserEvent:
			err = es.sendUserEvent(e)
		case *serf.Query:
			err = es.sendQuery(e)
		default:
			err = fmt.Errorf("Unknown event type: %s", event.EventType().String())
		}
		if err != nil {
			es.logger.Printf("[ERR] agent.ipc: Failed to stream event to %v: %v",
				es.client, err)
			return
		}
	}
}

// sendMemberEvent is used to send a single member event
func (es *eventStream) sendMemberEvent(me serf.MemberEvent) error {
	members := make([]Member, 0, len(me.Members))
	for _, m := range me.Members {
		sm := Member{
			Name:        m.Name,
			Addr:        m.Addr,
			Port:        m.Port,
			Tags:        m.Tags,
			Status:      m.Status.String(),
			ProtocolMin: m.ProtocolMin,
			ProtocolMax: m.ProtocolMax,
			ProtocolCur: m.ProtocolCur,
			DelegateMin: m.DelegateMin,
			DelegateMax: m.DelegateMax,
			DelegateCur: m.DelegateCur,
		}
		members = append(members, sm)
	}

	header := responseHeader{
		Seq:   es.seq,
		Error: "",
	}
	rec := memberEventRecord{
		Event:   me.String(),
		Members: members,
	}
	return es.client.Send(&header, &rec)
}

// sendUserEvent is used to send a single user event
func (es *eventStream) sendUserEvent(ue serf.UserEvent) error {
	header := responseHeader{
		Seq:   es.seq,
		Error: "",
	}
	rec := userEventRecord{
		Event:    ue.EventType().String(),
		LTime:    ue.LTime,
		Name:     ue.Name,
		Payload:  ue.Payload,
		Coalesce: ue.Coalesce,
	}
	return es.client.Send(&header, &rec)
}

// sendQuery is used to send a single query event
func (es *eventStream) sendQuery(q *serf.Query) error {
	id := es.client.RegisterQuery(q)

	header := responseHeader{
		Seq:   es.seq,
		Error: "",
	}
	rec := queryEventRecord{
		Event:   q.EventType().String(),
		ID:      id,
		LTime:   q.LTime,
		Name:    q.Name,
		Payload: q.Payload,
	}
	return es.client.Send(&header, &rec)
}
