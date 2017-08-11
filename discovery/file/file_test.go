// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package file

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
)

func TestFileSD(t *testing.T) {
	defer os.Remove("fixtures/_test_valid.yml")
	defer os.Remove("fixtures/_test_valid.json")
	defer os.Remove("fixtures/_test_invalid_nil.json")
	defer os.Remove("fixtures/_test_invalid_nil.yml")
	testFileSD(t, "valid", ".yml", true)
	testFileSD(t, "valid", ".json", true)
	testFileSD(t, "invalid_nil", ".json", false)
	testFileSD(t, "invalid_nil", ".yml", false)
}

func testFileSD(t *testing.T, prefix, ext string, expect bool) {
	// As interval refreshing is more of a fallback, we only want to test
	// whether file watches work as expected.
	var conf config.FileSDConfig
	conf.Files = []string{"fixtures/_*" + ext}
	conf.RefreshInterval = model.Duration(1 * time.Hour)

	var (
		fsd         = NewDiscovery(&conf, nil)
		ch          = make(chan []*config.TargetGroup)
		ctx, cancel = context.WithCancel(context.Background())
	)
	go fsd.Run(ctx, ch)

	select {
	case <-time.After(25 * time.Millisecond):
		// Expected.
	case tgs := <-ch:
		t.Fatalf("Unexpected target groups in file discovery: %s", tgs)
	}

	newf, err := os.Create("fixtures/_test_" + prefix + ext)
	if err != nil {
		t.Fatal(err)
	}
	defer newf.Close()

	f, err := os.Open("fixtures/" + prefix + ext)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	_, err = io.Copy(newf, f)
	if err != nil {
		t.Fatal(err)
	}
	newf.Close()

	timeout := time.After(15 * time.Second)
	// The files contain two target groups.
retry:
	for {
		select {
		case <-timeout:
			if expect {
				t.Fatalf("Expected new target group but got none")
			} else {
				// invalid type fsd should always broken down.
				break retry
			}
		case tgs := <-ch:
			if !expect {
				t.Fatalf("Unexpected target groups %s, we expected a failure here.", tgs)
			}

			if len(tgs) != 2 {
				continue retry // Potentially a partial write, just retry.
			}
			tg := tgs[0]

			if _, ok := tg.Labels["foo"]; !ok {
				t.Fatalf("Label not parsed")
			}
			if tg.String() != filepath.FromSlash(fmt.Sprintf("fixtures/_test_%s%s:0", prefix, ext)) {
				t.Fatalf("Unexpected target group %s", tg)
			}

			tg = tgs[1]
			if tg.String() != filepath.FromSlash(fmt.Sprintf("fixtures/_test_%s%s:1", prefix, ext)) {
				t.Fatalf("Unexpected target groups %s", tg)
			}
			break retry
		}
	}

	// Based on unknown circumstances, sometimes fsnotify will trigger more events in
	// some runs (which might be empty, chains of different operations etc.).
	// We have to drain those (as the target manager would) to avoid deadlocking and must
	// not try to make sense of it all...
	drained := make(chan struct{})
	go func() {
	Loop:
		for {
			select {
			case tgs := <-ch:
				// Below we will change the file to a bad syntax. Previously extracted target
				// groups must not be deleted via sending an empty target group.
				if len(tgs[0].Targets) == 0 {
					t.Errorf("Unexpected empty target groups received: %s", tgs)
				}
			case <-time.After(500 * time.Millisecond):
				break Loop
			}
		}
		close(drained)
	}()

	newf, err = os.Create("fixtures/_test.new")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(newf.Name())

	if _, err := newf.Write([]byte("]gibberish\n][")); err != nil {
		t.Fatal(err)
	}
	newf.Close()

	os.Rename(newf.Name(), "fixtures/_test_"+prefix+ext)

	cancel()
	<-drained
}
