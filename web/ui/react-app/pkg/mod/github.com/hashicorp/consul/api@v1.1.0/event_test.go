package api

import (
	"testing"

	"github.com/hashicorp/consul/sdk/testutil/retry"
)

func TestAPI_EventFireList(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	event := c.Event()

	params := &UserEvent{Name: "foo"}
	id, meta, err := event.Fire(params, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if meta.RequestTime == 0 {
		t.Fatalf("bad: %v", meta)
	}

	if id == "" {
		t.Fatalf("invalid: %v", id)
	}

	var events []*UserEvent
	var qm *QueryMeta

	retry.Run(t, func(r *retry.R) {
		events, qm, err = event.List("", nil)
		if err != nil {
			r.Fatalf("err: %v", err)
		}
		if len(events) <= 0 {
			r.Fatal(err)
		}
	})

	if events[len(events)-1].ID != id {
		t.Fatalf("bad: %#v", events)
	}

	if qm.LastIndex != event.IDToIndex(id) {
		t.Fatalf("Bad: %#v", qm)
	}
}
