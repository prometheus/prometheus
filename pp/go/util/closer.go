package util

type Closer struct {
	close  chan struct{}
	closed chan struct{}
}

func NewCloser() *Closer {
	return &Closer{
		close:  make(chan struct{}),
		closed: make(chan struct{}),
	}
}

func (c *Closer) Done() {
	close(c.closed)
}

func (c *Closer) Signal() <-chan struct{} {
	return c.close
}

func (c *Closer) Close() error {
	close(c.close)
	<-c.closed
	return nil
}
