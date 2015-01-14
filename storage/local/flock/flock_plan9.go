package flock

import "os"

type plan9Lock struct {
	f *os.File
}

func (l *plan9Lock) Release() error {
	return l.f.Close()
}

func newLock(fileName string) (Releaser, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, os.ModeExclusive|0644)
	if err != nil {
		return nil, err
	}
	return &plan9Lock{f}, nil
}
