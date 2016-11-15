// +build !windows,!plan9,!linux,!openbsd

package pages

// fdatasync flushes written data to a file descriptor.
func fdatasync(pb *DB) error {
	return pb.file.Sync()
}
