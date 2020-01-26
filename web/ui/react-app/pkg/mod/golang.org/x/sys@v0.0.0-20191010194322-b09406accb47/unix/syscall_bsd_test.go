// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd openbsd

package unix_test

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestGetfsstat(t *testing.T) {
	n, err := unix.Getfsstat(nil, unix.MNT_NOWAIT)
	if err != nil {
		t.Fatal(err)
	}

	data := make([]unix.Statfs_t, n)
	n2, err := unix.Getfsstat(data, unix.MNT_NOWAIT)
	if err != nil {
		t.Fatal(err)
	}
	if n != n2 {
		t.Errorf("Getfsstat(nil) = %d, but subsequent Getfsstat(slice) = %d", n, n2)
	}
	for i, stat := range data {
		if stat == (unix.Statfs_t{}) {
			t.Errorf("index %v is an empty Statfs_t struct", i)
		}
	}
	if t.Failed() {
		for i, stat := range data[:n2] {
			t.Logf("data[%v] = %+v", i, stat)
		}
		mount, err := exec.Command("mount").CombinedOutput()
		if err != nil {
			t.Logf("mount: %v\n%s", err, mount)
		} else {
			t.Logf("mount: %s", mount)
		}
	}
}

func TestSelect(t *testing.T) {
	n, err := unix.Select(0, nil, nil, nil, &unix.Timeval{Sec: 0, Usec: 0})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if n != 0 {
		t.Fatalf("Select: expected 0 ready file descriptors, got %v", n)
	}

	dur := 250 * time.Millisecond
	tv := unix.NsecToTimeval(int64(dur))
	start := time.Now()
	n, err = unix.Select(0, nil, nil, nil, &tv)
	took := time.Since(start)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if n != 0 {
		t.Fatalf("Select: expected 0 ready file descriptors, got %v", n)
	}

	// On some BSDs the actual timeout might also be slightly less than the requested.
	// Add an acceptable margin to avoid flaky tests.
	if took < dur*2/3 {
		t.Errorf("Select: timeout should have been at least %v, got %v", dur, took)
	}

	rr, ww, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer rr.Close()
	defer ww.Close()

	if _, err := ww.Write([]byte("HELLO GOPHER")); err != nil {
		t.Fatal(err)
	}

	rFdSet := &unix.FdSet{}
	fd := rr.Fd()
	// FD_SET(fd, rFdSet)
	rFdSet.Bits[fd/unix.NFDBITS] |= (1 << (fd % unix.NFDBITS))

	n, err = unix.Select(int(fd+1), rFdSet, nil, nil, nil)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if n != 1 {
		t.Fatalf("Select: expected 1 ready file descriptors, got %v", n)
	}
}

func TestSysctlRaw(t *testing.T) {
	if runtime.GOOS == "openbsd" {
		t.Skip("kern.proc.pid does not exist on OpenBSD")
	}

	_, err := unix.SysctlRaw("kern.proc.pid", unix.Getpid())
	if err != nil {
		t.Fatal(err)
	}
}

func TestSysctlUint32(t *testing.T) {
	maxproc, err := unix.SysctlUint32("kern.maxproc")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("kern.maxproc: %v", maxproc)
}
