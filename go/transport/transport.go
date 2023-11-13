package transport

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/util"
)

// Config - config for Transport.
type Config struct {
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// Transport - transport implementation.
type Transport struct {
	conn net.Conn
	cfg  *Config
}

// New - init new Transport with conn.
func New(cfg *Config, conn net.Conn) *Transport {
	return &Transport{
		conn: conn,
		cfg:  cfg,
	}
}

// Read - read msg from connection.
func (nt *Transport) Read(ctx context.Context) (*frames.ReadFrame, error) {
	if nt.conn == nil {
		return nil, net.ErrClosed
	}

	if err := nt.conn.SetReadDeadline(createDeadline(ctx, nt.cfg.ReadTimeout)); err != nil {
		return nil, fmt.Errorf("set read deadline: %w", err)
	}
	defer func() {
		if nt.conn != nil {
			_ = nt.conn.SetReadDeadline(time.Time{})
		}
	}()

	fe := frames.NewFrameEmpty()
	if err := fe.Read(ctx, nt.conn); err != nil {
		return nil, fmt.Errorf("read frame: %w", err)
	}

	return fe, nil
}

// write - write msg in connection.
func (nt *Transport) Write(ctx context.Context, fe *frames.ReadFrame) error {
	if nt.conn == nil {
		return net.ErrClosed
	}

	if err := nt.conn.SetWriteDeadline(createDeadline(ctx, nt.cfg.WriteTimeout)); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}
	defer func() {
		_ = nt.conn.SetWriteDeadline(time.Time{})
	}()

	if _, err := fe.WriteTo(nt.conn); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}

	return nil
}

// Writer returns new io.Writer with given context canceling
func (nt *Transport) Writer(ctx context.Context) io.Writer {
	return util.FnWriter(func(data []byte) (int, error) {
		if nt.conn == nil {
			return 0, net.ErrClosed
		}

		if err := nt.conn.SetWriteDeadline(createDeadline(ctx, nt.cfg.WriteTimeout)); err != nil {
			return 0, fmt.Errorf("set write deadline: %w", err)
		}
		defer func() {
			_ = nt.conn.SetWriteDeadline(time.Time{})
		}()

		return nt.conn.Write(data)
	})
}

// Close - close connection.
func (nt *Transport) Close() error {
	if nt.conn == nil {
		return nil
	}

	return nt.conn.Close()
}

// createDeadline - create deadline for conn from min ctx or timeout.
func createDeadline(ctx context.Context, timeout time.Duration) time.Time {
	deadline, ok := ctx.Deadline()
	if timeout == 0 {
		return deadline
	}

	timeoutDeadline := time.Now().Add(timeout)
	if !ok || deadline.After(timeoutDeadline) {
		return timeoutDeadline
	}

	return deadline
}
