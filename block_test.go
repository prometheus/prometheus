// Copyright 2017 The Prometheus Authors
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

package tsdb

import (
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/tsdb/labels"
	"github.com/prometheus/tsdb/testutil"
)

// In Prometheus 2.1.0 we had a bug where the meta.json version was falsely bumped
// to 2. We had a migration in place resetting it to 1 but we should move immediately to
// version 3 next time to avoid confusion and issues.
func TestBlockMetaMustNeverBeVersion2(t *testing.T) {
	dir, err := ioutil.TempDir("", "metaversion")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	testutil.Ok(t, writeMetaFile(dir, &BlockMeta{}))

	meta, err := readMetaFile(dir)
	testutil.Ok(t, err)
	testutil.Assert(t, meta.Version != 2, "meta.json version must never be 2")
}

func TestSetCompactionFailed(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test")
	testutil.Ok(t, err)
	defer os.RemoveAll(tmpdir)

	blockDir := createBlock(t, tmpdir, 1, 0, 0)
	b, err := OpenBlock(nil, blockDir, nil)
	testutil.Ok(t, err)
	testutil.Equals(t, false, b.meta.Compaction.Failed)
	testutil.Ok(t, b.setCompactionFailed())
	testutil.Equals(t, true, b.meta.Compaction.Failed)
	testutil.Ok(t, b.Close())

	b, err = OpenBlock(nil, blockDir, nil)
	testutil.Ok(t, err)
	testutil.Equals(t, true, b.meta.Compaction.Failed)
	testutil.Ok(t, b.Close())
}

// createBlock creates a block with nSeries series, filled with
// samples of the given mint,maxt time range and returns its dir.
func createBlock(tb testing.TB, dir string, nSeries int, mint, maxt int64) string {
	head, err := NewHead(nil, nil, nil, 2*60*60*1000)
	testutil.Ok(tb, err)
	defer head.Close()

	lbls, err := labels.ReadLabels(filepath.Join("testdata", "20kseries.json"), nSeries)
	testutil.Ok(tb, err)
	refs := make([]uint64, nSeries)

	for ts := mint; ts <= maxt; ts++ {
		app := head.Appender()
		for i, lbl := range lbls {
			if refs[i] != 0 {
				err := app.AddFast(refs[i], ts, rand.Float64())
				if err == nil {
					continue
				}
			}
			ref, err := app.Add(lbl, int64(ts), rand.Float64())
			testutil.Ok(tb, err)
			refs[i] = ref
		}
		err := app.Commit()
		testutil.Ok(tb, err)
	}

	compactor, err := NewLeveledCompactor(nil, log.NewNopLogger(), []int64{1000000}, nil)
	testutil.Ok(tb, err)

	testutil.Ok(tb, os.MkdirAll(dir, 0777))

	ulid, err := compactor.Write(dir, head, head.MinTime(), head.MaxTime(), nil)
	testutil.Ok(tb, err)
	return filepath.Join(dir, ulid.String())
}
