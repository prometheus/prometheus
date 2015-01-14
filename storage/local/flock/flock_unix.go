// +build darwin dragonfly freebsd linux netbsd openbsd

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
	how := syscall.LOCK_UN
	if lock {
		how = syscall.LOCK_EX
	}
	return syscall.Flock(int(l.f.Fd()), how|syscall.LOCK_NB)
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
