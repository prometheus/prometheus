package block

import "time"

type DelayedNoOpBlockWriter struct {
	delay time.Duration
}

func NewDelayedNoOpBlockWriter(delay time.Duration) *DelayedNoOpBlockWriter {
	return &DelayedNoOpBlockWriter{delay: delay}
}

func (w *DelayedNoOpBlockWriter) Write(_ Block) error {
	<-time.After(w.delay)
	return nil
}
