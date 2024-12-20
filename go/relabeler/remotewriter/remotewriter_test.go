package remotewriter

import (
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/ready"
	"github.com/prometheus/client_golang/prometheus"
	"testing"
)

func TestRemoteWriter_Run(t *testing.T) {
	rw := New("", nil, clockwork.NewFakeClock(), ready.NoOpNotifier{}, prometheus.DefaultRegisterer)
	_ = rw
}
