package tsdb

type ReloadBlocksExternalTrigger interface {
	Chan() <-chan struct{}
}

type noOpReloadBlocksExternalTrigger struct {
	c chan struct{}
}

func (t *noOpReloadBlocksExternalTrigger) Chan() <-chan struct{} {
	return t.c
}

func newNoOnReloadBlocksExternalTrigger() *noOpReloadBlocksExternalTrigger {
	return &noOpReloadBlocksExternalTrigger{c: make(chan struct{})}
}
