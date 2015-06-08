package flock

import "syscall"

type windowsLock struct {
	fd syscall.Handle
}

func (fl *windowsLock) Release() error {
	return syscall.Close(fl.fd)
}

func newLock(fileName string) (Releaser, error) {
	pathp, err := syscall.UTF16PtrFromString(fileName)
	if err != nil {
		return nil, err
	}
	fd, err := syscall.CreateFile(pathp, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.CREATE_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return &windowsLock{fd}, nil
}
