package framestest

import (
	"bytes"
	"context"
	"io"

	"github.com/prometheus/prometheus/pp/go/frames"
	"golang.org/x/sync/errgroup"
)

// ReadPayload reads payload to bytes
func ReadPayload(p frames.WritePayload) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, p.Size()))
	_, err := p.WriteTo(buf)
	return buf.Bytes(), err
}

// ReadFrame convert WriteFrame into ReadFrame
func ReadFrame(ctx context.Context, f *frames.WriteFrame) (*frames.ReadFrame, error) {
	r, w := io.Pipe()
	g := new(errgroup.Group)
	g.Go(func() error {
		_, err := f.WriteTo(w)
		return err
	})
	rf := frames.NewFrameEmpty()
	g.Go(func() error {
		return rf.Read(ctx, r)
	})
	return rf, g.Wait()
}
