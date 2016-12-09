// +build !windows,!plan9,!solaris

package tsdb

// import (
// 	"fmt"
// 	"syscall"
// 	"unsafe"
// )

// // mmap memory maps a DB's data file.
// func mmap(db *DB, sz int) error {
// 	// Map the data file to memory.
// 	b, err := syscall.Mmap(int(db.file.Fd()), 0, sz, syscall.PROT_READ, syscall.MAP_SHARED|db.MmapFlags)
// 	if err != nil {
// 		return err
// 	}

// 	// Advise the kernel that the mmap is accessed randomly.
// 	if err := madvise(b, syscall.MADV_RANDOM); err != nil {
// 		return fmt.Errorf("madvise: %s", err)
// 	}

// 	// Save the original byte slice and convert to a byte array pointer.
// 	db.dataref = b
// 	db.data = (*[maxMapSize]byte)(unsafe.Pointer(&b[0]))
// 	db.datasz = sz
// 	return nil
// }

// // munmap unmaps a DB's data file from memory.
// func munmap(db *DB) error {
// 	// Ignore the unmap if we have no mapped data.
// 	if db.dataref == nil {
// 		return nil
// 	}

// 	// Unmap using the original byte slice.
// 	err := syscall.Munmap(db.dataref)
// 	db.dataref = nil
// 	db.data = nil
// 	db.datasz = 0
// 	return err
// }

// // NOTE: This function is copied from stdlib because it is not available on darwin.
// func madvise(b []byte, advice int) (err error) {
// 	_, _, e1 := syscall.Syscall(syscall.SYS_MADVISE, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), uintptr(advice))
// 	if e1 != 0 {
// 		err = e1
// 	}
// 	return
// }
