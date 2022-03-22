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
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

const defaultWait = time.Second

type testRunner struct {
	*testing.T
	dir           string
	ch            chan []*targetgroup.Group
	done, stopped chan struct{}
	cancelSD      context.CancelFunc

	mtx        sync.Mutex
	tgs        map[string]*targetgroup.Group
	receivedAt time.Time
}

func newTestRunner(t *testing.T) *testRunner {
	t.Helper()

	return &testRunner{
		T:       t,
		dir:     t.TempDir(),
		ch:      make(chan []*targetgroup.Group),
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
		tgs:     make(map[string]*targetgroup.Group),
	}
}

// copyFile atomically copies a file to the runner's directory.
func (t *testRunner) copyFile(src string) string {
	t.Helper()
	return t.copyFileTo(src, filepath.Base(src))
}

// copyFileTo atomically copies a file with a different name to the runner's directory.
func (t *testRunner) copyFileTo(src, name string) string {
	t.Helper()

	newf, err := ioutil.TempFile(t.dir, "")
	require.NoError(t, err)

	f, err := os.Open(src)
	require.NoError(t, err)

	_, err = io.Copy(newf, f)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, newf.Close())

	dst := filepath.Join(t.dir, name)
	err = os.Rename(newf.Name(), dst)
	require.NoError(t, err)

	return dst
}

// writeString writes atomically a string to a file.
func (t *testRunner) writeString(file, data string) {
	t.Helper()

	newf, err := ioutil.TempFile(t.dir, "")
	require.NoError(t, err)

	_, err = newf.WriteString(data)
	require.NoError(t, err)
	require.NoError(t, newf.Close())

	err = os.Rename(newf.Name(), file)
	require.NoError(t, err)
}

// appendString appends a string to a file.
func (t *testRunner) appendString(file, data string) {
	t.Helper()

	f, err := os.OpenFile(file, os.O_WRONLY|os.O_APPEND, 0)
	require.NoError(t, err)
	defer f.Close()

	_, err = f.WriteString(data)
	require.NoError(t, err)
}

// run starts the file SD and the loop receiving target groups updates.
func (t *testRunner) run(files ...string) {
	go func() {
		defer close(t.stopped)
		for {
			select {
			case <-t.done:
				os.RemoveAll(t.dir)
				return
			case tgs := <-t.ch:
				t.mtx.Lock()
				t.receivedAt = time.Now()
				for _, tg := range tgs {
					t.tgs[tg.Source] = tg
				}
				t.mtx.Unlock()
			}
		}
	}()

	for i := range files {
		files[i] = filepath.Join(t.dir, files[i])
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.cancelSD = cancel
	go func() {
		NewDiscovery(
			&SDConfig{
				Files: files,
				// Setting a high refresh interval to make sure that the tests only
				// rely on file watches.
				RefreshInterval: model.Duration(1 * time.Hour),
			},
			nil,
		).Run(ctx, t.ch)
	}()
}

func (t *testRunner) stop() {
	t.cancelSD()
	close(t.done)
	<-t.stopped
}

func (t *testRunner) lastReceive() time.Time {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return t.receivedAt
}

func (t *testRunner) targets() []*targetgroup.Group {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	var (
		keys []string
		tgs  []*targetgroup.Group
	)

	for k := range t.tgs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		tgs = append(tgs, t.tgs[k])
	}
	return tgs
}

func (t *testRunner) requireUpdate(ref time.Time, expected []*targetgroup.Group) {
	t.Helper()

	for {
		select {
		case <-time.After(defaultWait):
			t.Fatalf("Expected update but got none")
			return
		case <-time.After(defaultWait / 10):
			if ref.Equal(t.lastReceive()) {
				// No update received.
				break
			}

			// We can receive partial updates so only check the result when the
			// expected number of groups is reached.
			tgs := t.targets()
			if len(tgs) != len(expected) {
				t.Logf("skipping update: expected %d targets, got %d", len(expected), len(tgs))
				break
			}
			t.requireTargetGroups(expected, tgs)
			if ref.After(time.Time{}) {
				t.Logf("update received after %v", t.lastReceive().Sub(ref))
			}
			return
		}
	}
}

func (t *testRunner) requireTargetGroups(expected, got []*targetgroup.Group) {
	t.Helper()
	b1, err := json.Marshal(expected)
	if err != nil {
		panic(err)
	}
	b2, err := json.Marshal(got)
	if err != nil {
		panic(err)
	}

	require.Equal(t, string(b1), string(b2))
}

// validTg() maps to fixtures/valid.{json,yml}.
func validTg(file string) []*targetgroup.Group {
	return []*targetgroup.Group{
		{
			Targets: []model.LabelSet{
				{
					model.AddressLabel: model.LabelValue("localhost:9090"),
				},
				{
					model.AddressLabel: model.LabelValue("example.org:443"),
				},
			},
			Labels: model.LabelSet{
				model.LabelName("foo"): model.LabelValue("bar"),
				fileSDFilepathLabel:    model.LabelValue(file),
			},
			Source: fileSource(file, 0),
		},
		{
			Targets: []model.LabelSet{
				{
					model.AddressLabel: model.LabelValue("my.domain"),
				},
			},
			Labels: model.LabelSet{
				fileSDFilepathLabel: model.LabelValue(file),
			},
			Source: fileSource(file, 1),
		},
	}
}

// valid2Tg() maps to fixtures/valid2.{json,yml}.
func valid2Tg(file string) []*targetgroup.Group {
	return []*targetgroup.Group{
		{
			Targets: []model.LabelSet{
				{
					model.AddressLabel: model.LabelValue("my.domain"),
				},
			},
			Labels: model.LabelSet{
				fileSDFilepathLabel: model.LabelValue(file),
			},
			Source: fileSource(file, 0),
		},
		{
			Targets: []model.LabelSet{
				{
					model.AddressLabel: model.LabelValue("localhost:9090"),
				},
			},
			Labels: model.LabelSet{
				model.LabelName("foo"):  model.LabelValue("bar"),
				model.LabelName("fred"): model.LabelValue("baz"),
				fileSDFilepathLabel:     model.LabelValue(file),
			},
			Source: fileSource(file, 1),
		},
		{
			Targets: []model.LabelSet{
				{
					model.AddressLabel: model.LabelValue("example.org:443"),
				},
			},
			Labels: model.LabelSet{
				model.LabelName("scheme"): model.LabelValue("https"),
				fileSDFilepathLabel:       model.LabelValue(file),
			},
			Source: fileSource(file, 2),
		},
	}
}

func TestInitialUpdate(t *testing.T) {
	for _, tc := range []string{
		"fixtures/valid.yml",
		"fixtures/valid.json",
	} {
		t.Run(tc, func(t *testing.T) {
			t.Parallel()

			runner := newTestRunner(t)
			sdFile := runner.copyFile(tc)

			runner.run("*" + filepath.Ext(tc))
			defer runner.stop()

			// Verify that we receive the initial target groups.
			runner.requireUpdate(time.Time{}, validTg(sdFile))
		})
	}
}

func TestInvalidFile(t *testing.T) {
	for _, tc := range []string{
		"fixtures/invalid_nil.yml",
		"fixtures/invalid_nil.json",
	} {
		tc := tc
		t.Run(tc, func(t *testing.T) {
			t.Parallel()

			now := time.Now()
			runner := newTestRunner(t)
			runner.copyFile(tc)

			runner.run("*" + filepath.Ext(tc))
			defer runner.stop()

			// Verify that we've received nothing.
			time.Sleep(defaultWait)
			if runner.lastReceive().After(now) {
				t.Fatalf("unexpected targets received: %v", runner.targets())
			}
		})
	}
}

func TestNoopFileUpdate(t *testing.T) {
	t.Parallel()

	runner := newTestRunner(t)
	sdFile := runner.copyFile("fixtures/valid.yml")

	runner.run("*.yml")
	defer runner.stop()

	// Verify that we receive the initial target groups.
	runner.requireUpdate(time.Time{}, validTg(sdFile))

	// Verify that we receive an update with the same target groups.
	ref := runner.lastReceive()
	runner.copyFileTo("fixtures/valid3.yml", "valid.yml")
	runner.requireUpdate(ref, validTg(sdFile))
}

func TestFileUpdate(t *testing.T) {
	t.Parallel()

	runner := newTestRunner(t)
	sdFile := runner.copyFile("fixtures/valid.yml")

	runner.run("*.yml")
	defer runner.stop()

	// Verify that we receive the initial target groups.
	runner.requireUpdate(time.Time{}, validTg(sdFile))

	// Verify that we receive an update with the new target groups.
	ref := runner.lastReceive()
	runner.copyFileTo("fixtures/valid2.yml", "valid.yml")
	runner.requireUpdate(ref, valid2Tg(sdFile))
}

func TestInvalidFileUpdate(t *testing.T) {
	t.Parallel()

	runner := newTestRunner(t)
	sdFile := runner.copyFile("fixtures/valid.yml")

	runner.run("*.yml")
	defer runner.stop()

	// Verify that we receive the initial target groups.
	runner.requireUpdate(time.Time{}, validTg(sdFile))

	ref := runner.lastReceive()
	runner.writeString(sdFile, "]gibberish\n][")

	// Verify that we receive nothing or the same targets as before.
	time.Sleep(defaultWait)
	if runner.lastReceive().After(ref) {
		runner.requireTargetGroups(validTg(sdFile), runner.targets())
	}
}

func TestUpdateFileWithPartialWrites(t *testing.T) {
	t.Parallel()

	runner := newTestRunner(t)
	sdFile := runner.copyFile("fixtures/valid.yml")

	runner.run("*.yml")
	defer runner.stop()

	// Verify that we receive the initial target groups.
	runner.requireUpdate(time.Time{}, validTg(sdFile))

	// Do a partial write operation.
	ref := runner.lastReceive()
	runner.writeString(sdFile, "- targets")
	time.Sleep(defaultWait)
	// Verify that we receive nothing or the same target groups as before.
	if runner.lastReceive().After(ref) {
		runner.requireTargetGroups(validTg(sdFile), runner.targets())
	}

	// Verify that we receive the update target groups once the file is a valid YAML payload.
	ref = runner.lastReceive()
	runner.appendString(sdFile, `: ["localhost:9091"]`)
	runner.requireUpdate(ref,
		[]*targetgroup.Group{
			{
				Targets: []model.LabelSet{
					{
						model.AddressLabel: model.LabelValue("localhost:9091"),
					},
				},
				Labels: model.LabelSet{
					fileSDFilepathLabel: model.LabelValue(sdFile),
				},
				Source: fileSource(sdFile, 0),
			},
			{
				Source: fileSource(sdFile, 1),
			},
		},
	)
}

func TestRemoveFile(t *testing.T) {
	t.Parallel()

	runner := newTestRunner(t)
	sdFile := runner.copyFile("fixtures/valid.yml")

	runner.run("*.yml")
	defer runner.stop()

	// Verify that we receive the initial target groups.
	runner.requireUpdate(time.Time{}, validTg(sdFile))

	// Verify that we receive the update about the target groups being removed.
	ref := runner.lastReceive()
	require.NoError(t, os.Remove(sdFile))
	runner.requireUpdate(
		ref,
		[]*targetgroup.Group{
			{
				Source: fileSource(sdFile, 0),
			},
			{
				Source: fileSource(sdFile, 1),
			},
		},
	)
}
