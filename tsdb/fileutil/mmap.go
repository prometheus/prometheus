// Copyright 2018 The Prometheus Authors
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
	"fmt"
	"os"
	"runtime"
)

type mmapRef struct {
	b []byte
}

func (m *mmapRef) close() error {
	if m.b == nil {
		return fmt.Errorf("mmap already closed")
	}
	err := munmap(m.b)
	m.b = nil
	return err
}

type MmapFile struct {
	f *os.File
	m *mmapRef
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

	mmapFile := &MmapFile{f: f, m: &mmapRef{b: b}}

	runtime.AddCleanup(mmapFile, func(m *mmapRef) {
		_ = m.close()
	}, mmapFile.m)

	return mmapFile, nil
}

func (f *MmapFile) Close() error {
	err0 := f.m.close()
	err1 := f.f.Close()

	if err0 != nil {
		return err0
	}
	return err1
}

func (f *MmapFile) File() *os.File {
	return f.f
}

func (f *MmapFile) Bytes() []byte {
	return f.m.b
}
