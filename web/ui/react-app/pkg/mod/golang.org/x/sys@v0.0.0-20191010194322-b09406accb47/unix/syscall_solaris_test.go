// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build solaris

package unix_test

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSelect(t *testing.T) {
	n, err := unix.Select(0, nil, nil, nil, &unix.Timeval{Sec: 0, Usec: 0})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if n != 0 {
		t.Fatalf("Select: expected 0 ready file descriptors, got %v", n)
	}

	dur := 150 * time.Millisecond
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

	if took < dur {
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

func TestStatvfs(t *testing.T) {
	if err := unix.Statvfs("", nil); err == nil {
		t.Fatal(`Statvfs("") expected failure`)
	}

	statvfs := unix.Statvfs_t{}
	if err := unix.Statvfs("/", &statvfs); err != nil {
		t.Errorf(`Statvfs("/") failed: %v`, err)
	}

	if t.Failed() {
		mount, err := exec.Command("mount").CombinedOutput()
		if err != nil {
			t.Logf("mount: %v\n%s", err, mount)
		} else {
			t.Logf("mount: %s", mount)
		}
	}
}
