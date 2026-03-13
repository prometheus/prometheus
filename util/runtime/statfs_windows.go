// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build windows

package runtime

import (
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	dll                 = windows.MustLoadDLL("kernel32.dll")
	getDiskFreeSpaceExW = dll.MustFindProc("GetDiskFreeSpaceExW")
)

func FsType(path string) string {
	return "unknown"
}

func FsSize(path string) uint64 {
	// Ensure the path exists.
	if _, err := os.Stat(path); err != nil {
		return 0
	}

	utf16Path, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0
	}

	var avail int64
	var total int64
	var free int64
	// https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getdiskfreespaceexa
	ret, _, _ := getDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(utf16Path)),
		uintptr(unsafe.Pointer(&avail)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&free)))

	if ret == 0 || uint64(free) > uint64(total) {
		return 0
	}

	return uint64(total)
}
