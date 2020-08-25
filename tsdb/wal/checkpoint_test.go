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

package wal

import (
	"fmt"
	"github.com/go-kit/kit/log"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestLastCheckpoint(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_checkpoint")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	_, _, err = LastCheckpoint(dir)
	testutil.Equals(t, record.ErrNotFound, err)

	testutil.Ok(t, os.MkdirAll(filepath.Join(dir, "checkpoint.0000"), 0777))
	s, k, err := LastCheckpoint(dir)
	testutil.Ok(t, err)
	testutil.Equals(t, filepath.Join(dir, "checkpoint.0000"), s)
	testutil.Equals(t, 0, k)

	testutil.Ok(t, os.MkdirAll(filepath.Join(dir, "checkpoint.xyz"), 0777))
	s, k, err = LastCheckpoint(dir)
	testutil.Ok(t, err)
	testutil.Equals(t, filepath.Join(dir, "checkpoint.0000"), s)
	testutil.Equals(t, 0, k)

	testutil.Ok(t, os.MkdirAll(filepath.Join(dir, "checkpoint.1"), 0777))
	s, k, err = LastCheckpoint(dir)
	testutil.Ok(t, err)
	testutil.Equals(t, filepath.Join(dir, "checkpoint.1"), s)
	testutil.Equals(t, 1, k)

	testutil.Ok(t, os.MkdirAll(filepath.Join(dir, "checkpoint.1000"), 0777))
	s, k, err = LastCheckpoint(dir)
	testutil.Ok(t, err)
	testutil.Equals(t, filepath.Join(dir, "checkpoint.1000"), s)
	testutil.Equals(t, 1000, k)

	testutil.Ok(t, os.MkdirAll(filepath.Join(dir, "checkpoint.99999999"), 0777))
	s, k, err = LastCheckpoint(dir)
	testutil.Ok(t, err)
	testutil.Equals(t, filepath.Join(dir, "checkpoint.99999999"), s)
	testutil.Equals(t, 99999999, k)

	testutil.Ok(t, os.MkdirAll(filepath.Join(dir, "checkpoint.100000000"), 0777))
	s, k, err = LastCheckpoint(dir)
	testutil.Ok(t, err)
	testutil.Equals(t, filepath.Join(dir, "checkpoint.100000000"), s)
	testutil.Equals(t, 100000000, k)
}

func TestDeleteCheckpoints(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_checkpoint")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	testutil.Ok(t, DeleteCheckpoints(dir, 0))

	testutil.Ok(t, os.MkdirAll(filepath.Join(dir, "checkpoint.00"), 0777))
	testutil.Ok(t, os.MkdirAll(filepath.Join(dir, "checkpoint.01"), 0777))
	testutil.Ok(t, os.MkdirAll(filepath.Join(dir, "checkpoint.02"), 0777))
	testutil.Ok(t, os.MkdirAll(filepath.Join(dir, "checkpoint.03"), 0777))

	testutil.Ok(t, DeleteCheckpoints(dir, 2))

	files, err := ioutil.ReadDir(dir)
	testutil.Ok(t, err)
	fns := []string{}
	for _, f := range files {
		fns = append(fns, f.Name())
	}
	testutil.Equals(t, []string{"checkpoint.02", "checkpoint.03"}, fns)

	testutil.Ok(t, os.MkdirAll(filepath.Join(dir, "checkpoint.99999999"), 0777))
	testutil.Ok(t, os.MkdirAll(filepath.Join(dir, "checkpoint.100000000"), 0777))
	testutil.Ok(t, os.MkdirAll(filepath.Join(dir, "checkpoint.100000001"), 0777))

	testutil.Ok(t, DeleteCheckpoints(dir, 100000000))

	files, err = ioutil.ReadDir(dir)
	testutil.Ok(t, err)
	fns = []string{}
	for _, f := range files {
		fns = append(fns, f.Name())
	}
	testutil.Equals(t, []string{"checkpoint.100000000", "checkpoint.100000001"}, fns)
}

func TestCheckpoint(t *testing.T) {
	for _, compress := range []bool{false, true} {
		t.Run(fmt.Sprintf("compress=%t", compress), func(t *testing.T) {
			dir, err := ioutil.TempDir("", "test_checkpoint")
			testutil.Ok(t, err)
			defer func() {
				testutil.Ok(t, os.RemoveAll(dir))
			}()

			var enc record.Encoder
			// Create a dummy segment to bump the initial number.
			seg, err := CreateSegment(dir, 100)
			testutil.Ok(t, err)
			testutil.Ok(t, seg.Close())

			// Manually create checkpoint for 99 and earlier.
			w, err := New(nil, nil, filepath.Join(dir, "checkpoint.0099"), compress)
			testutil.Ok(t, err)

			// Add some data we expect to be around later.
			err = w.Log(enc.Series([]record.RefSeries{
				{Ref: 0, Labels: labels.FromStrings("a", "b", "c", "0")},
				{Ref: 1, Labels: labels.FromStrings("a", "b", "c", "1")},
			}, nil))
			testutil.Ok(t, err)
			testutil.Ok(t, w.Close())

			// Start a WAL and write records to it as usual.
			w, err = NewSize(nil, nil, dir, 64*1024, compress)
			testutil.Ok(t, err)

			var last int64
			for i := 0; ; i++ {
				_, n, err := w.Segments()
				testutil.Ok(t, err)
				if n >= 106 {
					break
				}
				// Write some series initially.
				if i == 0 {
					b := enc.Series([]record.RefSeries{
						{Ref: 2, Labels: labels.FromStrings("a", "b", "c", "2")},
						{Ref: 3, Labels: labels.FromStrings("a", "b", "c", "3")},
						{Ref: 4, Labels: labels.FromStrings("a", "b", "c", "4")},
						{Ref: 5, Labels: labels.FromStrings("a", "b", "c", "5")},
					}, nil)
					testutil.Ok(t, w.Log(b))
				}
				// Write samples until the WAL has enough segments.
				// Make them have drifting timestamps within a record to see that they
				// get filtered properly.
				b := enc.Samples([]record.RefSample{
					{Ref: 0, T: last, V: float64(i)},
					{Ref: 1, T: last + 10000, V: float64(i)},
					{Ref: 2, T: last + 20000, V: float64(i)},
					{Ref: 3, T: last + 30000, V: float64(i)},
				}, nil)
				testutil.Ok(t, w.Log(b))

				last += 100
			}
			testutil.Ok(t, w.Close())

			_, err = Checkpoint(log.NewNopLogger(), w, 100, 106, func(x uint64) bool {
				return x%2 == 0
			}, last/2)
			testutil.Ok(t, err)
			testutil.Ok(t, w.Truncate(107))
			testutil.Ok(t, DeleteCheckpoints(w.Dir(), 106))

			// Only the new checkpoint should be left.
			files, err := ioutil.ReadDir(dir)
			testutil.Ok(t, err)
			testutil.Equals(t, 1, len(files))
			testutil.Equals(t, "checkpoint.00000106", files[0].Name())

			sr, err := NewSegmentsReader(filepath.Join(dir, "checkpoint.00000106"))
			testutil.Ok(t, err)
			defer sr.Close()

			var dec record.Decoder
			var series []record.RefSeries
			r := NewReader(sr)

			for r.Next() {
				rec := r.Record()

				switch dec.Type(rec) {
				case record.Series:
					series, err = dec.Series(rec, series)
					testutil.Ok(t, err)
				case record.Samples:
					samples, err := dec.Samples(rec, nil)
					testutil.Ok(t, err)
					for _, s := range samples {
						testutil.Assert(t, s.T >= last/2, "sample with wrong timestamp")
					}
				}
			}
			testutil.Ok(t, r.Err())
			testutil.Equals(t, []record.RefSeries{
				{Ref: 0, Labels: labels.FromStrings("a", "b", "c", "0")},
				{Ref: 2, Labels: labels.FromStrings("a", "b", "c", "2")},
				{Ref: 4, Labels: labels.FromStrings("a", "b", "c", "4")},
			}, series)
		})
	}
}

func TestCheckpointNoTmpFolderAfterError(t *testing.T) {
	// Create a new wal with an invalid records.
	dir, err := ioutil.TempDir("", "test_checkpoint")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()
	w, err := NewSize(nil, nil, dir, 64*1024, false)
	testutil.Ok(t, err)
	testutil.Ok(t, w.Log([]byte{99}))
	w.Close()

	// Run the checkpoint and since the wal contains an invalid records this should return an error.
	_, err = Checkpoint(log.NewNopLogger(), w, 0, 1, nil, 0)
	testutil.NotOk(t, err)

	// Walk the wal dir to make sure there are no tmp folder left behind after the error.
	err = filepath.Walk(w.Dir(), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.Wrapf(err, "access err %q: %v\n", path, err)
		}
		if info.IsDir() && strings.HasSuffix(info.Name(), ".tmp") {
			return fmt.Errorf("wal dir contains temporary folder:%s", info.Name())
		}
		return nil
	})
	testutil.Ok(t, err)
}
