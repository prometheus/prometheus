// Copyright 2021 The Prometheus Authors
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

package fileutil

import (
	"errors"
	"io"
	"os"
	"sync"
)

type MmapWriter struct {
	sync.Mutex
	f    *os.File
	mf   *MmapFile
	buf  []byte
	wpos int
	rpos int
}

func NewMmapWriter(f *os.File) *MmapWriter {
	return &MmapWriter{f: f}
}

func NewMmapWriterWithSize(f *os.File, size int) (*MmapWriter, error) {
	mw := NewMmapWriter(f)
	err := mw.mmap(size)
	return mw, err
}

func (mw *MmapWriter) Bytes() []byte {
	return mw.buf
}

func (mw *MmapWriter) Sync() error {
	if mw.mf != nil {
		return mw.mf.Sync()
	}
	return nil
}

func (mw *MmapWriter) Close() error {
	mw.buf = nil
	if mw.mf != nil {
		return mw.mf.Close()
	}
	return nil
}

func (mw *MmapWriter) mmap(size int) error {
	err := mw.f.Truncate(int64(size))
	if err != nil {
		return err
	}
	mf, err := OpenRwMmapFromFile(mw.f, size)
	if err != nil {
		return err
	}
	mw.mf = mf
	mw.buf = mf.Bytes()
	return nil
}

func (mw *MmapWriter) resize(size int) error {
	err := mw.f.Truncate(int64(size))
	if err != nil {
		return err
	}
	err = mw.mf.resize(size)
	if err != nil {
		return err
	}
	mw.buf = mw.mf.Bytes()
	return nil
}

func (mw *MmapWriter) Seek(offset int64, whence int) (ret int64, err error) {
	var abs int
	switch whence {
	case io.SeekStart:
		abs = int(offset)
	default:
		return 0, errors.New("invalid whence")
	}
	if abs < 0 {
		return 0, errors.New("negative position")
	}
	mw.Lock()
	defer mw.Unlock()
	mw.rpos = abs
	return offset, nil
}

func (mw *MmapWriter) Read(p []byte) (n int, err error) {
	mw.Lock()
	defer mw.Unlock()
	if mw.rpos >= len(mw.buf) {
		return 0, io.EOF
	}
	n = copy(p, mw.buf[mw.rpos:])
	mw.rpos += n
	return
}

func (mw *MmapWriter) Write(p []byte) (n int, err error) {
	mw.Lock()
	defer mw.Unlock()
	if mw.mf == nil {
		err = mw.mmap(len(p))
		if err != nil {
			return
		}
	}
	if len(p) > len(mw.buf)-mw.wpos {
		err = mw.resize(mw.wpos + len(p))
		if err != nil {
			return
		}
	}

	n = copy(mw.buf[mw.wpos:], p)
	mw.wpos += n
	err = mw.Sync()
	return
}

func (mw *MmapWriter) WriteAt(p []byte, pos int64) (n int, err error) {
	mw.Lock()
	defer mw.Unlock()
	if mw.mf == nil {
		err = mw.mmap(len(p) + int(pos))
		if err != nil {
			return
		}
	}
	if len(p)+int(pos) > len(mw.buf) {
		err = mw.resize(len(p) + int(pos))
		if err != nil {
			return
		}
	}
	n = copy(mw.buf[pos:], p)
	err = mw.Sync()
	return
}
