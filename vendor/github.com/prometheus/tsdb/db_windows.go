package tsdb

import (
	"os"
	"syscall"
	"unsafe"
)

func mmap(f *os.File, sz int) ([]byte, error) {
	low, high := uint32(sz), uint32(sz>>32)

	h, errno := syscall.CreateFileMapping(syscall.Handle(f.Fd()), nil, syscall.PAGE_READONLY, low, high, nil)
	if h == 0 {
		return nil, os.NewSyscallError("CreateFileMapping", errno)
	}

	addr, errno := syscall.MapViewOfFile(h, syscall.FILE_MAP_READ, 0, 0, uintptr(sz))
	if addr == 0 {
		return nil, os.NewSyscallError("MapViewOfFile", errno)
	}

	if err := syscall.CloseHandle(syscall.Handle(h)); err != nil {
		return nil, os.NewSyscallError("CloseHandle", err)
	}

	return (*[1 << 30]byte)(unsafe.Pointer(addr))[:sz], nil
}

func munmap(b []byte) error {
	if err := syscall.UnmapViewOfFile((uintptr)(unsafe.Pointer(&b[0]))); err != nil {
		return os.NewSyscallError("UnmapViewOfFile", err)
	}
	return nil
}
