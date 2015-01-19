// +build solaris

package flock

import (
	"os"
	"syscall"
)

type unixLock struct {
	f *os.File
}

func (l *unixLock) Release() error {
	if err := l.set(false); err != nil {
		return err
	}
	return l.f.Close()
}

func (l *unixLock) set(lock bool) error {
	flock := syscall.Flock_t{
		Type:   syscall.F_UNLCK,
		Start:  0,
		Len:    0,
		Whence: 1,
	}
	if lock {
		flock.Type = syscall.F_WRLCK
	}
	return syscall.FcntlFlock(l.f.Fd(), syscall.F_SETLK, &flock)
}

func newLock(fileName string) (Releaser, error) {
	f, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	l := &unixLock{f}
	err = l.set(true)
	if err != nil {
		f.Close()
		return nil, err
	}
	return l, nil
}
