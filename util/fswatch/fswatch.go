package fswatch

import (
	"context"
	"errors"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
)

type Watch struct {
	name     string
	logger   log.Logger
	interval time.Duration
	delay    time.Duration

	// TODO: Close it, handle it.
	notifyCh chan struct{}
}

func New(
	reg prometheus.Registerer,
	name string,
	logger log.Logger,
	interval time.Duration,
	delay time.Duration,
) *Watch {
	// TODO: Add metrics

	return &Watch{name: name, logger: logger, interval: interval, delay: delay, notifyCh: make(chan struct{}, 1)}
}

// TODO(bwplotka): In future we could consider string slice channel to mention which files changed. For now
// reload does not care (it reloads all).
// Only one caller at the time can use this.
func (w *Watch) FilesChanged() <-chan struct{} {
	if w == nil {
		return nil // A receive from a nil channel blocks forever.
	}
	return w.notifyCh
}

func (w *Watch) Name() string { return w.name }

func (w *Watch) AddFiles(ctx context.Context, file ...string) error {
	return errors.New("not implemented")
}

func (w *Watch) Reset(ctx context.Context) error {
	return errors.New("not implemented")
}

// Run errors means run stopped working, it should be safe to restart by calling Run again.
func (w *Watch) Run(ctx context.Context) error {
	// TODO: Copy the code from https://github.com/thanos-io/thanos/blob/main/pkg/reloader/reloader.go and adjust based on requirements
	return errors.New("not implemented")
}
