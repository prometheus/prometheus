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
	"os"

	"github.com/pkg/errors"
)

type MmapFile struct {
	f         *os.File
	b         []byte
	rw        bool
	closeFile bool
}

func OpenRwMmapFromFile(f *os.File, size int) (mf *MmapFile, retErr error) {
	defer func() {
		if retErr != nil {
			f.Close()
		}
	}()
	if size <= 0 {
		info, err := f.Stat()
		if err != nil {
			return nil, errors.Wrap(err, "stat")
		}
		size = int(info.Size())
	}

	b, err := mmapRw(f, size)
	if err != nil {
		return nil, errors.Wrapf(err, "mmap, size %d", size)
	}
	return &MmapFile{f: f, b: b, rw: true}, nil
}

func OpenMmapFile(path string) (*MmapFile, error) {
	return OpenMmapFileWithSize(path, 0)
}

func OpenMmapFileWithSize(path string, size int) (mf *MmapFile, retErr error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "try lock file")
	}
	defer func() {
		if retErr != nil {
			f.Close()
		}
	}()
	if size <= 0 {
		info, err := f.Stat()
		if err != nil {
			return nil, errors.Wrap(err, "stat")
		}
		size = int(info.Size())
	}

	b, err := mmapRo(f, size)
	if err != nil {
		return nil, errors.Wrapf(err, "mmap, size %d", size)
	}
	return &MmapFile{f: f, b: b, closeFile: true}, nil
}

func (f *MmapFile) resize(size int) error {
	err := f.Sync()
	if err != nil {
		return errors.Wrap(err, "resize sync")
	}
	err = munmap(f.b)
	if err != nil {
		return errors.Wrap(err, "resize munmap")
	}
	var b []byte
	if f.rw {
		b, err = mmapRw(f.f, size)
	} else {
		b, err = mmapRo(f.f, size)
	}
	if err != nil {
		return errors.Wrap(err, "resize mmap")
	}
	f.b = b
	return nil
}

func (f *MmapFile) Close() error {
	err0 := f.Sync()
	err1 := munmap(f.b)
	var err2 error
	if f.closeFile {
		err2 = f.f.Close()
	}

	if err0 != nil {
		return errors.Wrap(err0, "close sync")
	}
	if err1 != nil {
		return errors.Wrap(err1, "close munmap")
	}
	if err2 != nil {
		return errors.Wrap(err2, "close file")
	}
	return nil
}

func (f *MmapFile) File() *os.File {
	return f.f
}

func (f *MmapFile) Bytes() []byte {
	return f.b
}
