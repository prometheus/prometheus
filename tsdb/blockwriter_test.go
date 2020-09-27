// Copyright 2020 The Prometheus Authors
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
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestBlockWriter(t *testing.T) {
	ctx := context.Background()
	outputDir, err := ioutil.TempDir(os.TempDir(), "output")
	testutil.Ok(t, err)
	w := NewBlockWriter(log.NewNopLogger(), outputDir, DefaultBlockDuration)
	testutil.Ok(t, w.initHead())

	// flush with no series results in error
	_, err = w.Flush(ctx)
	testutil.ErrorEqual(t, err, errors.New("no series appended, aborting"))

	// add some series
	app := w.Appender(ctx)
	expectedLabelNames := "a"
	l := labels.Labels{{Name: expectedLabelNames, Value: "b"}}
	ts := int64(44)
	_, err = app.Add(l, ts, 7)
	testutil.Ok(t, err)
	err = app.Commit()
	testutil.Ok(t, err)
	id, err := w.Flush(ctx)
	testutil.Ok(t, err)

	// confirm block has the correct data
	blockpath := filepath.Join(outputDir, id.String())
	b, err := OpenBlock(nil, blockpath, nil)
	testutil.Ok(t, err)
	testutil.Equals(t, b.MinTime(), ts)
	testutil.Equals(t, b.MaxTime(), ts+int64(1))
	actualNames, err := b.LabelNames()
	testutil.Ok(t, err)
	testutil.Equals(t, actualNames, []string{expectedLabelNames})

	testutil.Ok(t, w.Close())
}
