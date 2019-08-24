// Copyright 2011 Evan Shaw. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mmap

import (
	"errors"
	"os"
	"sync"

	"golang.org/x/sys/windows"
)

// mmap on Windows is a two-step process.
// First, we call CreateFileMapping to get a handle.
// Then, we call MapviewToFile to get an actual pointer into memory.
// Because we want to emulate a POSIX-style mmap, we don't want to expose
// the handle -- only the pointer. We also want to return only a byte slice,
// not a struct, so it's convenient to manipulate.

// We keep this map so that we can get back the original handle from the memory address.

type addrinfo struct {
	file    windows.Handle
	mapview windows.Handle
}

var handleLock sync.Mutex
var handleMap = map[uintptr]*addrinfo{}

func mmap(len int, prot, flags, hfile uintptr, off int64) ([]byte, error) {
	flProtect := uint32(windows.PAGE_READONLY)
	dwDesiredAccess := uint32(windows.FILE_MAP_READ)
	switch {
	case prot&COPY != 0:
		flProtect = windows.PAGE_WRITECOPY
		dwDesiredAccess = windows.FILE_MAP_COPY
	case prot&RDWR != 0:
		flProtect = windows.PAGE_READWRITE
		dwDesiredAccess = windows.FILE_MAP_WRITE
	}
	if prot&EXEC != 0 {
		flProtect <<= 4
		dwDesiredAccess |= windows.FILE_MAP_EXECUTE
	}

	// The maximum size is the area of the file, starting from 0,
	// that we wish to allow to be mappable. It is the sum of
	// the length the user requested, plus the offset where that length
	// is starting from. This does not map the data into memory.
	maxSizeHigh := uint32((off + int64(len)) >> 32)
	maxSizeLow := uint32((off + int64(len)) & 0xFFFFFFFF)
	// TODO: Do we need to set some security attributes? It might help portability.
	h, errno := windows.CreateFileMapping(windows.Handle(hfile), nil, flProtect, maxSizeHigh, maxSizeLow, nil)
	if h == 0 {
		return nil, os.NewSyscallError("CreateFileMapping", errno)
	}

	// Actually map a view of the data into memory. The view's size
	// is the length the user requested.
	fileOffsetHigh := uint32(off >> 32)
	fileOffsetLow := uint32(off & 0xFFFFFFFF)
	addr, errno := windows.MapViewOfFile(h, dwDesiredAccess, fileOffsetHigh, fileOffsetLow, uintptr(len))
	if addr == 0 {
		return nil, os.NewSyscallError("MapViewOfFile", errno)
	}
	handleLock.Lock()
	handleMap[addr] = &addrinfo{
		file:    windows.Handle(hfile),
		mapview: h,
	}
	handleLock.Unlock()

	m := MMap{}
	dh := m.header()
	dh.Data = addr
	dh.Len = len
	dh.Cap = dh.Len

	return m, nil
}

func (m MMap) flush() error {
	addr, len := m.addrLen()
	errno := windows.FlushViewOfFile(addr, len)
	if errno != nil {
		return os.NewSyscallError("FlushViewOfFile", errno)
	}

	handleLock.Lock()
	defer handleLock.Unlock()
	handle, ok := handleMap[addr]
	if !ok {
		// should be impossible; we would've errored above
		return errors.New("unknown base address")
	}

	errno = windows.FlushFileBuffers(handle.file)
	return os.NewSyscallError("FlushFileBuffers", errno)
}

func (m MMap) lock() error {
	addr, len := m.addrLen()
	errno := windows.VirtualLock(addr, len)
	return os.NewSyscallError("VirtualLock", errno)
}

func (m MMap) unlock() error {
	addr, len := m.addrLen()
	errno := windows.VirtualUnlock(addr, len)
	return os.NewSyscallError("VirtualUnlock", errno)
}

func (m MMap) unmap() error {
	err := m.flush()
	if err != nil {
		return err
	}

	addr := m.header().Data
	// Lock the UnmapViewOfFile along with the handleMap deletion.
	// As soon as we unmap the view, the OS is free to give the
	// same addr to another new map. We don't want another goroutine
	// to insert and remove the same addr into handleMap while
	// we're trying to remove our old addr/handle pair.
	handleLock.Lock()
	defer handleLock.Unlock()
	err = windows.UnmapViewOfFile(addr)
	if err != nil {
		return err
	}

	handle, ok := handleMap[addr]
	if !ok {
		// should be impossible; we would've errored above
		return errors.New("unknown base address")
	}
	delete(handleMap, addr)

	e := windows.CloseHandle(windows.Handle(handle.mapview))
	return os.NewSyscallError("CloseHandle", e)
}
