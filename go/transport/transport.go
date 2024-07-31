package transport

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/util"
	"golang.org/x/net/websocket"
)

const (
	defaultReadTimeout  = 0 * time.Second
	defaultWriteTimeout = 300 * time.Second
)

// Config - config for Transport.
type Config struct {
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// DefaultConfig generate default config for Transport.
func DefaultConfig() *Config {
	return &Config{
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
	}
}

// NetTransport - transport implementation.
type NetTransport struct {
	conn net.Conn
	cfg  *Config
}

// NewNetTransport - init new Transport with conn.
func NewNetTransport(cfg *Config, conn net.Conn) *NetTransport {
	return &NetTransport{
		conn: conn,
		cfg:  cfg,
	}
}

// Read - read msg from connection.
func (nt *NetTransport) Read(ctx context.Context) (*frames.ReadFrame, error) {
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
func (nt *NetTransport) Write(ctx context.Context, fe *frames.ReadFrame) error {
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
func (nt *NetTransport) Writer(ctx context.Context) io.Writer {
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
func (nt *NetTransport) Close() error {
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

// WebSocketTransport - transport implementation.
type WebSocketTransport struct {
	conn *websocket.Conn
	cfg  *Config
}

// NewWebSocketTransport - init new Transport with conn.
func NewWebSocketTransport(cfg *Config, wsconn *websocket.Conn) *WebSocketTransport {
	wsconn.PayloadType = websocket.BinaryFrame
	return &WebSocketTransport{
		conn: wsconn,
		cfg:  cfg,
	}
}

// Read - read msg from connection.
func (wst *WebSocketTransport) Read(ctx context.Context, fr frames.FrameReader) error {
	if wst.conn == nil {
		return net.ErrClosed
	}

	if err := wst.conn.SetReadDeadline(createDeadline(ctx, wst.cfg.ReadTimeout)); err != nil {
		return fmt.Errorf("set read deadline: %w", err)
	}
	defer func() {
		if wst.conn != nil {
			_ = wst.conn.SetReadDeadline(time.Time{})
		}
	}()

	if err := fr.Read(ctx, wst.conn); err != nil {
		return fmt.Errorf("read frame: %w", err)
	}

	return nil
}

// write - write msg in connection.
func (wst *WebSocketTransport) Write(ctx context.Context, fw frames.FrameWriter) error {
	if wst.conn == nil {
		return net.ErrClosed
	}

	if err := wst.conn.SetWriteDeadline(createDeadline(ctx, wst.cfg.WriteTimeout)); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}
	defer func() {
		_ = wst.conn.SetWriteDeadline(time.Time{})
	}()

	if _, err := fw.WriteTo(wst.conn); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}

	return nil
}

// Writer - returns new io.Writer with given context canceling.
func (wst *WebSocketTransport) Writer(ctx context.Context) io.Writer {
	return util.FnWriter(func(data []byte) (int, error) {
		if wst.conn == nil {
			return 0, net.ErrClosed
		}

		if err := wst.conn.SetWriteDeadline(createDeadline(ctx, wst.cfg.WriteTimeout)); err != nil {
			return 0, fmt.Errorf("set write deadline: %w", err)
		}
		defer func() {
			_ = wst.conn.SetWriteDeadline(time.Time{})
		}()

		return wst.conn.Write(data)
	})
}

// Close - close connection.
func (wst *WebSocketTransport) Close() error {
	if wst.conn == nil {
		return nil
	}

	return wst.conn.Close()
}
