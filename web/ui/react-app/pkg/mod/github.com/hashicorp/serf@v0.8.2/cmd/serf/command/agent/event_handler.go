package agent

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/hashicorp/serf/serf"
)

// EventHandler is a handler that does things when events happen.
type EventHandler interface {
	HandleEvent(serf.Event)
}

// ScriptEventHandler invokes scripts for the events that it receives.
type ScriptEventHandler struct {
	SelfFunc func() serf.Member
	Scripts  []EventScript
	Logger   *log.Logger

	scriptLock sync.Mutex
	newScripts []EventScript
}

func (h *ScriptEventHandler) HandleEvent(e serf.Event) {
	// Swap in the new scripts if any
	h.scriptLock.Lock()
	if h.newScripts != nil {
		h.Scripts = h.newScripts
		h.newScripts = nil
	}
	h.scriptLock.Unlock()

	if h.Logger == nil {
		h.Logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	self := h.SelfFunc()
	for _, script := range h.Scripts {
		if !script.Invoke(e) {
			continue
		}

		err := invokeEventScript(h.Logger, script.Script, self, e)
		if err != nil {
			h.Logger.Printf("[ERR] agent: Error invoking script '%s': %s",
				script.Script, err)
		}
	}
}

// UpdateScripts is used to safely update the scripts we invoke in
// a thread safe manner
func (h *ScriptEventHandler) UpdateScripts(scripts []EventScript) {
	h.scriptLock.Lock()
	defer h.scriptLock.Unlock()
	h.newScripts = scripts
}

// EventFilter is used to filter which events are processed
type EventFilter struct {
	Event string
	Name  string
}

// Invoke tests whether or not this event script should be invoked
// for the given Serf event.
func (s *EventFilter) Invoke(e serf.Event) bool {
	if s.Event == "*" {
		return true
	}

	if e.EventType().String() != s.Event {
		return false
	}

	if s.Event == "user" && s.Name != "" {
		userE, ok := e.(serf.UserEvent)
		if !ok {
			return false
		}

		if userE.Name != s.Name {
			return false
		}
	}

	if s.Event == "query" && s.Name != "" {
		query, ok := e.(*serf.Query)
		if !ok {
			return false
		}

		if query.Name != s.Name {
			return false
		}
	}

	return true
}

// Valid checks if this is a valid agent event script.
func (s *EventFilter) Valid() bool {
	switch s.Event {
	case "member-join":
	case "member-leave":
	case "member-failed":
	case "member-update":
	case "member-reap":
	case "user":
	case "query":
	case "*":
	default:
		return false
	}
	return true
}

// EventScript is a single event script that will be executed in the
// case of an event, and is configured from the command-line or from
// a configuration file.
type EventScript struct {
	EventFilter
	Script string
}

func (s *EventScript) String() string {
	if s.Name != "" {
		return fmt.Sprintf("Event '%s:%s' invoking '%s'", s.Event, s.Name, s.Script)
	}
	return fmt.Sprintf("Event '%s' invoking '%s'", s.Event, s.Script)
}

// ParseEventScript takes a string in the format of "type=script" and
// parses it into an EventScript struct, if it can.
func ParseEventScript(v string) []EventScript {
	var filter, script string
	parts := strings.SplitN(v, "=", 2)
	if len(parts) == 1 {
		script = parts[0]
	} else {
		filter = parts[0]
		script = parts[1]
	}

	filters := ParseEventFilter(filter)
	results := make([]EventScript, 0, len(filters))
	for _, filt := range filters {
		result := EventScript{
			EventFilter: filt,
			Script:      script,
		}
		results = append(results, result)
	}
	return results
}

// ParseEventFilter a string with the event type filters and
// parses it into a series of EventFilters if it can.
func ParseEventFilter(v string) []EventFilter {
	// No filter translates to stream all
	if v == "" {
		v = "*"
	}

	events := strings.Split(v, ",")
	results := make([]EventFilter, 0, len(events))
	for _, event := range events {
		var result EventFilter
		var name string

		if strings.HasPrefix(event, "user:") {
			name = event[len("user:"):]
			event = "user"
		} else if strings.HasPrefix(event, "query:") {
			name = event[len("query:"):]
			event = "query"
		}

		result.Event = event
		result.Name = name
		results = append(results, result)
	}

	return results
}
