// +build !windows,!plan9,!solaris

package tsdb

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

func mmap(f *os.File, length int) ([]byte, error) {
	return unix.Mmap(int(f.Fd()), 0, length, unix.PROT_READ, unix.MAP_SHARED)
}

func munmap(b []byte) (err error) {
	return unix.Munmap(b)
}

// unix.Madvise is not defined for darwin, so we define it ourselves.
func madvise(b []byte, advice int) (err error) {
	_, _, e1 := unix.Syscall(unix.SYS_MADVISE, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), uintptr(advice))
	if e1 != 0 {
		err = e1
	}
	return
}
