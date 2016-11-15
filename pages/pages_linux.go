package pages

import (
	"syscall"
)

// fdatasync flushes written data to a file descriptor.
func fdatasync(pb *Pagebuf) error {
	return syscall.Fdatasync(int(pb.file.Fd()))
}
