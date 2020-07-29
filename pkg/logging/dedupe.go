// Copyright 2019 The Prometheus Authors
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

package logging

import (
	"bytes"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-logfmt/logfmt"
)

const (
	garbageCollectEvery = 10 * time.Second
	expireEntriesAfter  = 1 * time.Minute
	maxEntries          = 1024
)

type logfmtEncoder struct {
	*logfmt.Encoder
	buf bytes.Buffer
}

var logfmtEncoderPool = sync.Pool{
	New: func() interface{} {
		var enc logfmtEncoder
		enc.Encoder = logfmt.NewEncoder(&enc.buf)
		return &enc
	},
}

// Deduper implement log.Logger, dedupes log lines.
type Deduper struct {
	next   log.Logger
	repeat time.Duration
	quit   chan struct{}
	mtx    sync.RWMutex
	seen   map[string]time.Time
}

// Dedupe log lines to next, only repeating every repeat duration.
func Dedupe(next log.Logger, repeat time.Duration) *Deduper {
	d := &Deduper{
		next:   next,
		repeat: repeat,
		quit:   make(chan struct{}),
		seen:   map[string]time.Time{},
	}
	go d.run()
	return d
}

// Stop the Deduper.
func (d *Deduper) Stop() {
	close(d.quit)
}

func (d *Deduper) run() {
	ticker := time.NewTicker(garbageCollectEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.mtx.Lock()
			now := time.Now()
			for line, seen := range d.seen {
				if now.Sub(seen) > expireEntriesAfter {
					delete(d.seen, line)
				}
			}
			d.mtx.Unlock()
		case <-d.quit:
			return
		}
	}
}

// Log implements log.Logger.
func (d *Deduper) Log(keyvals ...interface{}) error {
	line, err := encode(keyvals...)
	if err != nil {
		return err
	}

	d.mtx.RLock()
	last, ok := d.seen[line]
	d.mtx.RUnlock()

	if ok && time.Since(last) < d.repeat {
		return nil
	}

	d.mtx.Lock()
	if len(d.seen) < maxEntries {
		d.seen[line] = time.Now()
	}
	d.mtx.Unlock()

	return d.next.Log(keyvals...)
}

func encode(keyvals ...interface{}) (string, error) {
	enc := logfmtEncoderPool.Get().(*logfmtEncoder)
	enc.buf.Reset()
	defer logfmtEncoderPool.Put(enc)

	if err := enc.EncodeKeyvals(keyvals...); err != nil {
		return "", err
	}

	// Add newline to the end of the buffer
	if err := enc.EndRecord(); err != nil {
		return "", err
	}

	return enc.buf.String(), nil
}
