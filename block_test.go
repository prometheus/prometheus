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
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/tsdb/index"
	"github.com/prometheus/tsdb/testutil"
)

func TestSetCompactionFailed(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test-tsdb")
	testutil.Ok(t, err)

	b := createEmptyBlock(t, tmpdir)

	testutil.Equals(t, false, b.meta.Compaction.Failed)
	testutil.Ok(t, b.setCompactionFailed())
	testutil.Equals(t, true, b.meta.Compaction.Failed)
	testutil.Ok(t, b.Close())

	b, err = OpenBlock(tmpdir, nil)
	testutil.Ok(t, err)
	testutil.Equals(t, true, b.meta.Compaction.Failed)
}

func createEmptyBlock(t *testing.T, dir string) *Block {
	testutil.Ok(t, os.MkdirAll(dir, 0777))

	testutil.Ok(t, writeMetaFile(dir, &BlockMeta{Version: 2}))

	ir, err := index.NewWriter(filepath.Join(dir, indexFilename))
	testutil.Ok(t, err)
	testutil.Ok(t, ir.Close())

	testutil.Ok(t, os.MkdirAll(chunkDir(dir), 0777))

	testutil.Ok(t, writeTombstoneFile(dir, EmptyTombstoneReader()))

	b, err := OpenBlock(dir, nil)
	testutil.Ok(t, err)
	return b
}
