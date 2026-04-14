// Copyright The Prometheus Authors
// Based on golang.org/x/net/netutil:
//   Copyright 2013 The Go Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package netconnlimit provides network utility functions for limiting
// simultaneous connections across multiple listeners.
package netconnlimit

import (
	"net"
	"sync"
)

// NewSharedSemaphore creates and returns a new semaphore channel that can be used
// to limit the number of simultaneous connections across multiple listeners.
func NewSharedSemaphore(n int) chan struct{} {
	return make(chan struct{}, n)
}

// SharedLimitListener returns a listener that accepts at most n simultaneous
// connections across multiple listeners using the provided shared semaphore.
func SharedLimitListener(l net.Listener, sem chan struct{}) net.Listener {
	return &sharedLimitListener{
		Listener: l,
		sem:      sem,
		done:     make(chan struct{}),
	}
}

type sharedLimitListener struct {
	net.Listener
	sem       chan struct{}
	closeOnce sync.Once     // Ensures the done chan is only closed once.
	done      chan struct{} // No values sent; closed when Close is called.
}

// Acquire acquires the shared semaphore. Returns true if successfully
// acquired, false if the listener is closed and the semaphore is not
// acquired.
func (l *sharedLimitListener) acquire() bool {
	select {
	case <-l.done:
		return false
	case l.sem <- struct{}{}:
		return true
	}
}

func (l *sharedLimitListener) release() { <-l.sem }

func (l *sharedLimitListener) Accept() (net.Conn, error) {
	if !l.acquire() {
		for {
			c, err := l.Listener.Accept()
			if err != nil {
				return nil, err
			}
			c.Close()
		}
	}

	c, err := l.Listener.Accept()
	if err != nil {
		l.release()
		return nil, err
	}
	return &sharedLimitListenerConn{Conn: c, release: l.release}, nil
}

func (l *sharedLimitListener) Close() error {
	err := l.Listener.Close()
	l.closeOnce.Do(func() { close(l.done) })
	return err
}

type sharedLimitListenerConn struct {
	net.Conn
	releaseOnce sync.Once
	release     func()
}

func (l *sharedLimitListenerConn) Close() error {
	err := l.Conn.Close()
	l.releaseOnce.Do(l.release)
	return err
}
