package agent

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/logutils"
)

func TestIPCLogStream(t *testing.T) {
	sc := &MockStreamClient{}
	filter := LevelFilter()
	filter.MinLevel = logutils.LogLevel("INFO")

	ls := newLogStream(sc, filter, 42, log.New(os.Stderr, "", log.LstdFlags))
	defer ls.Stop()

	log := "[DEBUG] this is a test log"
	log2 := "[INFO] This should pass"
	ls.HandleLog(log)
	ls.HandleLog(log2)

	time.Sleep(5 * time.Millisecond)

	if len(sc.headers) != 1 {
		t.Fatalf("expected 1 messages!")
	}
	for _, h := range sc.headers {
		if h.Seq != 42 {
			t.Fatalf("bad seq")
		}
		if h.Error != "" {
			t.Fatalf("bad err")
		}
	}

	obj1 := sc.objs[0].(*logRecord)
	if obj1.Log != log2 {
		t.Fatalf("bad event %#v", obj1)
	}
}
