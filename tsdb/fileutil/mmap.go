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

package fileutil

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/prometheus/prometheus/tsdb/encoding"
)

type mmapRef struct {
	b []byte
}

func (m *mmapRef) close() error {
	return m.closeWith(munmap)
}

func (m *mmapRef) closeWith(unmap func([]byte) error) error {
	if m.b == nil {
		return errors.New("mmap already closed")
	}
	err := unmap(m.b)
	if err != nil {
		return err
	}
	m.b = nil
	return nil
}

type MmapFile struct {
	f       *os.File
	m       *mmapRef
	cleanup runtime.Cleanup
}

func OpenMmapFile(path string) (*MmapFile, error) {
	return OpenMmapFileWithSize(path, 0)
}

func OpenMmapFileWithSize(path string, size int) (mf *MmapFile, retErr error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("try lock file: %w", err)
	}
	defer func() {
		if retErr != nil {
			f.Close()
		}
	}()
	if size <= 0 {
		info, err := f.Stat()
		if err != nil {
			return nil, fmt.Errorf("stat: %w", err)
		}
		size = int(info.Size())
	}

	b, err := mmap(f, size)
	if err != nil {
		return nil, fmt.Errorf("mmap, size %d: %w", size, err)
	}

	m := &mmapRef{b: b}
	mmapFile := &MmapFile{f: f, m: m}
	mmapFile.cleanup = runtime.AddCleanup(m, func(b []byte) {
		_ = munmap(b)
	}, b)

	return mmapFile, nil
}

// Close invalidates all views returned by Bytes and BytesView.
func (f *MmapFile) Close() error {
	err0 := f.m.close()
	if err0 == nil {
		f.cleanup.Stop()
	}
	err1 := f.f.Close()
	runtime.KeepAlive(f.m)

	if err0 != nil {
		return err0
	}
	return err1
}

func (f *MmapFile) File() *os.File {
	return f.f
}

// Bytes returns a borrowed view of the mapping. The caller must keep f
// reachable until it finishes using the slice. The slice is invalid after Close.
func (f *MmapFile) Bytes() []byte {
	b := f.m.b
	runtime.KeepAlive(f)
	return b
}

// BytesView returns a view that keeps the mapping reachable. It is invalid
// after Close.
func (f *MmapFile) BytesView() encoding.ByteView {
	return encoding.NewByteViewWithOwner(f.m.b, f.m)
}
