package remotewriter

import (
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/ready"
	"testing"
)

func TestRemoteWriter_Run(t *testing.T) {
	rw := New(nil, clockwork.NewFakeClock(), ready.NoOpNotifier{})
	_ = rw
}
