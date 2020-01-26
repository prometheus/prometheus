package agent

import (
	"sync"

	"github.com/hashicorp/serf/serf"
)

// MockEventHandler is an EventHandler implementation that can be used
// for tests.
type MockEventHandler struct {
	Events []serf.Event
	sync.Mutex
}

func (h *MockEventHandler) HandleEvent(e serf.Event) {
	h.Lock()
	defer h.Unlock()
	h.Events = append(h.Events, e)
}

// MockQueryHandler is an EventHandler implementation used for tests,
// it always responds to a query with a given response
type MockQueryHandler struct {
	Response []byte
	Queries  []*serf.Query
	sync.Mutex
}

func (h *MockQueryHandler) HandleEvent(e serf.Event) {
	query, ok := e.(*serf.Query)
	if !ok {
		return
	}

	h.Lock()
	h.Queries = append(h.Queries, query)
	h.Unlock()

	query.Respond(h.Response)
}
