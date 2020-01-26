// Copyright 2019 The Prometheus Authors
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

func TestSoftnet(t *testing.T) {
	fs, err := NewFS(procTestFixtures)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := fs.GatherSoftnetStats()
	if err != nil {
		t.Fatal(err)
	}

	if want, got := uint(0x00015c73), entries[0].Processed; want != got {
		t.Errorf("want %08x, got %08x", want, got)
	}

	if want, got := uint(0x00020e76), entries[0].Dropped; want != got {
		t.Errorf("want %08x, got %08x", want, got)
	}

	if want, got := uint(0xF0000769), entries[0].TimeSqueezed; want != got {
		t.Errorf("want %08x, got %08x", want, got)
	}
}
