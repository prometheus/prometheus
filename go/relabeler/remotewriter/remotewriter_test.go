package remotewriter

import (
	"github.com/jonboulle/clockwork"
	"testing"
)

func TestRemoteWriter_Run(t *testing.T) {
	rw := New(nil, clockwork.NewFakeClock())
	_ = rw
}
