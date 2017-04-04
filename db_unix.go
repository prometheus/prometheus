// +build !windows,!plan9,!solaris

package tsdb

import (
	"os"
	"syscall"
)

func mmap(f *os.File, length int) ([]byte, error) {
	return syscall.Mmap(int(f.Fd()), 0, length, syscall.PROT_READ, syscall.MAP_SHARED)
}

func munmap(b []byte) (err error) {
	return syscall.Munmap(b)
}
