// Copyright 2018 The Prometheus Authors
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

package procfs

import (
	"testing"
)

func TestNewNetUnix(t *testing.T) {
	fs, err := NewFS(procTestFixtures)
	if err != nil {
		t.Fatal(err)
	}

	nu, err := fs.NewNetUnix()
	if err != nil {
		t.Fatal(err)
	}

	lines := []*NetUnixLine{
		&NetUnixLine{
			KernelPtr: "0000000000000000",
			RefCount:  2,
			Flags:     1 << 16,
			Type:      1,
			State:     1,
			Inode:     3442596,
			Path:      "/var/run/postgresql/.s.PGSQL.5432",
		},
		&NetUnixLine{
			KernelPtr: "0000000000000000",
			RefCount:  10,
			Flags:     1 << 16,
			Type:      5,
			State:     1,
			Inode:     10061,
			Path:      "/run/udev/control",
		},
		&NetUnixLine{
			KernelPtr: "0000000000000000",
			RefCount:  7,
			Flags:     0,
			Type:      2,
			State:     1,
			Inode:     12392,
			Path:      "/dev/log",
		},
		&NetUnixLine{
			KernelPtr: "0000000000000000",
			RefCount:  3,
			Flags:     0,
			Type:      1,
			State:     3,
			Inode:     4787297,
			Path:      "/var/run/postgresql/.s.PGSQL.5432",
		},
		&NetUnixLine{
			KernelPtr: "0000000000000000",
			RefCount:  3,
			Flags:     0,
			Type:      1,
			State:     3,
			Inode:     5091797,
		},
	}

	if want, have := len(lines), len(nu.Rows); want != have {
		t.Errorf("want %d parsed net/unix lines, have %d", want, have)
	}
	for i, gotLine := range nu.Rows {
		if i >= len(lines) {
			continue
		}
		line := lines[i]
		if *line != *gotLine {
			t.Errorf("%d item: got %v, want %v", i, *gotLine, *line)
		}
	}

	wantedFlags := "listen"
	flags := lines[0].Flags.String()
	if wantedFlags != flags {
		t.Errorf("unexpected flag str: want %s, got %s", wantedFlags, flags)
	}
	wantedFlags = "default"
	flags = lines[3].Flags.String()
	if wantedFlags != flags {
		t.Errorf("unexpected flag str: wanted %s, got %s", wantedFlags, flags)
	}

	wantedType := "stream"
	typ := lines[0].Type.String()
	if wantedType != typ {
		t.Errorf("unexpected type str: wanted %s, got %s", wantedType, typ)
	}

	wantedType = "seqpacket"
	typ = lines[1].Type.String()
	if wantedType != typ {
		t.Errorf("unexpected type str: want %s, got %s", wantedType, typ)
	}
}

func TestNewNetUnixWithoutInode(t *testing.T) {
	fs, err := NewFS(procTestFixtures)
	if err != nil {
		t.Fatal(err)
	}
	nu, err := NewNetUnixByPath(fs.proc.Path("net/unix_without_inode"))
	if err != nil {
		t.Fatal(err)
	}

	lines := []*NetUnixLine{
		&NetUnixLine{
			KernelPtr: "0000000000000000",
			RefCount:  2,
			Flags:     1 << 16,
			Type:      1,
			State:     1,
			Path:      "/var/run/postgresql/.s.PGSQL.5432",
		},
		&NetUnixLine{
			KernelPtr: "0000000000000000",
			RefCount:  10,
			Flags:     1 << 16,
			Type:      5,
			State:     1,
			Path:      "/run/udev/control",
		},
		&NetUnixLine{
			KernelPtr: "0000000000000000",
			RefCount:  7,
			Flags:     0,
			Type:      2,
			State:     1,
			Path:      "/dev/log",
		},
		&NetUnixLine{
			KernelPtr: "0000000000000000",
			RefCount:  3,
			Flags:     0,
			Type:      1,
			State:     3,
			Path:      "/var/run/postgresql/.s.PGSQL.5432",
		},
		&NetUnixLine{
			KernelPtr: "0000000000000000",
			RefCount:  3,
			Flags:     0,
			Type:      1,
			State:     3,
		},
	}

	if want, have := len(lines), len(nu.Rows); want != have {
		t.Errorf("want %d parsed net/unix lines, have %d", want, have)
	}
	for i, gotLine := range nu.Rows {
		if i >= len(lines) {
			continue
		}
		line := lines[i]
		if *line != *gotLine {
			t.Errorf("%d item: got %v, want %v", i, *gotLine, *line)
		}
	}
}
