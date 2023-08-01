package transport

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/prometheus/prometheus/pp/go/frames"
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
func (nt *Transport) Read(ctx context.Context) (*frames.Frame, error) {
	if nt.conn == nil {
		return nil, net.ErrClosed
	}

	if nt.cfg.ReadTimeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, nt.cfg.ReadTimeout)
		defer cancel()
	}

	deadline, _ := ctx.Deadline()
	if err := nt.conn.SetReadDeadline(deadline); err != nil {
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
func (nt *Transport) Write(ctx context.Context, fe *frames.Frame) error {
	if nt.conn == nil {
		return net.ErrClosed
	}

	if nt.cfg.WriteTimeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, nt.cfg.WriteTimeout)
		defer cancel()
	}

	deadline, _ := ctx.Deadline()
	if err := nt.conn.SetWriteDeadline(deadline); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}
	defer func() {
		_ = nt.conn.SetWriteDeadline(time.Time{})
	}()

	if err := fe.Write(ctx, nt.conn); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}

	return nil
}

// Close - close connection.
func (nt *Transport) Close() error {
	if nt.conn == nil {
		return nil
	}

	return nt.conn.Close()
}
