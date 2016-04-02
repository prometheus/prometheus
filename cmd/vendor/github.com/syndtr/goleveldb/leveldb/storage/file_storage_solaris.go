// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build solaris

package storage

import (
	"os"
	"syscall"
)

type unixFileLock struct {
	f *os.File
}

func (fl *unixFileLock) release() error {
	if err := setFileLock(fl.f, false); err != nil {
		return err
	}
	return fl.f.Close()
}

func newFileLock(path string) (fl fileLock, err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	err = setFileLock(f, true)
	if err != nil {
		f.Close()
		return
	}
	fl = &unixFileLock{f: f}
	return
}

func setFileLock(f *os.File, lock bool) error {
	flock := syscall.Flock_t{
		Type:   syscall.F_UNLCK,
		Start:  0,
		Len:    0,
		Whence: 1,
	}
	if lock {
		flock.Type = syscall.F_WRLCK
	}
	return syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &flock)
}

func rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func syncDir(name string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return err
	}
	return nil
}
