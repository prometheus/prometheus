// Copyright 2016 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package conntrack

import (
	"fmt"
	"net"

	"sync"

	"golang.org/x/net/trace"
	"time"
)

const (
	defaultName = "default"
)

type listenerOpts struct {
	name         string
	monitoring   bool
	tracing      bool
	tcpKeepAlive time.Duration
}

type listenerOpt func(*listenerOpts)

// TrackWithName sets the name of the Listener for use in tracking and monitoring.
func TrackWithName(name string) listenerOpt {
	return func(opts *listenerOpts) {
		opts.name = name
	}
}

// TrackWithoutMonitoring turns *off* Prometheus monitoring for this listener.
func TrackWithoutMonitoring() listenerOpt {
	return func(opts *listenerOpts) {
		opts.monitoring = false
	}
}

// TrackWithTracing turns *on* the /debug/events tracing of the live listener connections.
func TrackWithTracing() listenerOpt {
	return func(opts *listenerOpts) {
		opts.tracing = true
	}
}

// TrackWithTcpKeepAlive makes sure that any `net.TCPConn` that get accepted have a keep-alive.
// This is useful for HTTP servers in order for, for example laptops, to not use up resources on the
// server while they don't utilise their connection.
// A value of 0 disables it.
func TrackWithTcpKeepAlive(keepalive time.Duration) listenerOpt {
	return func(opts *listenerOpts) {
		opts.tcpKeepAlive = keepalive
	}
}

type connTrackListener struct {
	net.Listener
	opts *listenerOpts
}

// NewListener returns the given listener wrapped in connection tracking listener.
func NewListener(inner net.Listener, optFuncs ...listenerOpt) net.Listener {
	opts := &listenerOpts{
		name:       defaultName,
		monitoring: true,
		tracing:    false,
	}
	for _, f := range optFuncs {
		f(opts)
	}
	if opts.monitoring {
		preRegisterListenerMetrics(opts.name)
	}
	return &connTrackListener{
		Listener: inner,
		opts:     opts,
	}
}

func (ct *connTrackListener) Accept() (net.Conn, error) {
	// TODO(mwitkow): Add monitoring of failed accept.
	conn, err := ct.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok && ct.opts.tcpKeepAlive > 0 {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(ct.opts.tcpKeepAlive)
	}
	return newServerConnTracker(conn, ct.opts), nil
}

type serverConnTracker struct {
	net.Conn
	opts  *listenerOpts
	event trace.EventLog
	mu    sync.Mutex
}

func newServerConnTracker(inner net.Conn, opts *listenerOpts) net.Conn {

	tracker := &serverConnTracker{
		Conn: inner,
		opts: opts,
	}
	if opts.tracing {
		tracker.event = trace.NewEventLog(fmt.Sprintf("net.ServerConn.%s", opts.name), fmt.Sprintf("%v", inner.RemoteAddr()))
		tracker.event.Printf("accepted: %v -> %v", inner.RemoteAddr(), inner.LocalAddr())
	}
	if opts.monitoring {
		reportListenerConnAccepted(opts.name)
	}
	return tracker
}

func (ct *serverConnTracker) Close() error {
	err := ct.Conn.Close()
	ct.mu.Lock()
	if ct.event != nil {
		if err != nil {
			ct.event.Errorf("failed closing: %v", err)
		} else {
			ct.event.Printf("closing")
		}
		ct.event.Finish()
		ct.event = nil
	}
	ct.mu.Unlock()
	if ct.opts.monitoring {
		reportListenerConnClosed(ct.opts.name)
	}
	return err
}
