// Copyright The Prometheus Authors
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

package compression

import (
	"sync"

	"github.com/klauspost/compress/zstd"
)

type EncodeBuffer interface {
	zstdEncBuf() *zstd.Encoder
	get() []byte
	set([]byte)
}

type syncEBuffer struct {
	onceZstd sync.Once
	w        *zstd.Encoder
	buf      []byte
}

// NewSyncEncodeBuffer returns synchronous buffer that can only be used
// on one encoding goroutine at once. Notably, the encoded byte slice returned
// by Encode is valid only until the next Encode call.
func NewSyncEncodeBuffer() EncodeBuffer {
	return &syncEBuffer{}
}

func (b *syncEBuffer) zstdEncBuf() *zstd.Encoder {
	b.onceZstd.Do(func() {
		// Without params this never returns error.
		b.w, _ = zstd.NewWriter(nil)
	})
	return b.w
}

func (b *syncEBuffer) get() []byte {
	return b.buf
}

func (b *syncEBuffer) set(buf []byte) {
	b.buf = buf
}

type concurrentEBuffer struct {
	onceZstd sync.Once
	w        *zstd.Encoder
}

// NewConcurrentEncodeBuffer returns a buffer that can be used concurrently.
// NOTE: For Zstd compression, a concurrency limit equal to GOMAXPROCS is implied.
func NewConcurrentEncodeBuffer() EncodeBuffer {
	return &concurrentEBuffer{}
}

func (b *concurrentEBuffer) zstdEncBuf() *zstd.Encoder {
	b.onceZstd.Do(func() {
		// Without params this never returns error.
		b.w, _ = zstd.NewWriter(nil)
	})
	return b.w
}

// TODO(bwplotka): We could use pool, but putting it back into the pool needs to be
// on the caller side, so no pool for now.
func (*concurrentEBuffer) get() []byte {
	return nil
}

func (*concurrentEBuffer) set([]byte) {}

type DecodeBuffer interface {
	zstdDecBuf() *zstd.Decoder
	get() []byte
	set([]byte)
}

type syncDBuffer struct {
	onceZstd sync.Once
	r        *zstd.Decoder
	buf      []byte
}

// NewSyncDecodeBuffer returns synchronous buffer that can only be used
// on one decoding goroutine at once. Notably, the decoded byte slice returned
// by Decode is valid only until the next Decode call.
func NewSyncDecodeBuffer() DecodeBuffer {
	return &syncDBuffer{}
}

func (b *syncDBuffer) zstdDecBuf() *zstd.Decoder {
	b.onceZstd.Do(func() {
		// Without params this never returns error.
		b.r, _ = zstd.NewReader(nil)
	})
	return b.r
}

func (b *syncDBuffer) get() []byte {
	return b.buf
}

func (b *syncDBuffer) set(buf []byte) {
	b.buf = buf
}

type concurrentDBuffer struct {
	onceZstd sync.Once
	r        *zstd.Decoder
}

// NewConcurrentDecodeBuffer returns a buffer that can be used concurrently.
// NOTE: For Zstd compression a concurrency limit, equal to GOMAXPROCS is implied.
func NewConcurrentDecodeBuffer() DecodeBuffer {
	return &concurrentDBuffer{}
}

func (b *concurrentDBuffer) zstdDecBuf() *zstd.Decoder {
	b.onceZstd.Do(func() {
		// Without params this never returns error.
		b.r, _ = zstd.NewReader(nil)
	})
	return b.r
}

func (*concurrentDBuffer) get() []byte {
	return nil
}

func (*concurrentDBuffer) set([]byte) {}
