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
	"testing"

	"github.com/prometheus/tsdb/labels"
)

func TestDataAvailableOnlyAfterCommit(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	db, err := Open(tmpdir, nil, nil, nil)
	if err != nil {
		t.Fatalf("Error opening database: %q", err)
	}
	defer db.Close()

	app := db.Appender()
	_, err = app.Add(labels.FromStrings("foo", "bar"), 0, 0)
	if err != nil {
		t.Fatalf("Error adding sample: %q", err)
	}

	querier := db.Querier(0, 1)
	defer querier.Close()
	seriesSet := querier.Select(labels.NewEqualMatcher("foo", "bar"))
	if seriesSet.Next() {
		t.Fatalf("Error, was expecting no matching series.")
	}

	err = app.Commit()
	if err != nil {
		t.Fatalf("Error committing: %q", err)
	}

	querier = db.Querier(0, 1)
	defer querier.Close()

	seriesSet = querier.Select(labels.NewEqualMatcher("foo", "bar"))
	if !seriesSet.Next() {
		t.Fatalf("Error, was expecting a matching series.")
	}
}

func TestDataNotAvailableAfterRollback(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	db, err := Open(tmpdir, nil, nil, nil)
	if err != nil {
		t.Fatalf("Error opening database: %q", err)
	}
	defer db.Close()

	app := db.Appender()
	_, err = app.Add(labels.FromStrings("foo", "bar"), 0, 0)
	if err != nil {
		t.Fatalf("Error adding sample: %q", err)
	}

	err = app.Rollback()
	if err != nil {
		t.Fatalf("Error rolling back: %q", err)
	}

	querier := db.Querier(0, 1)
	defer querier.Close()

	seriesSet := querier.Select(labels.NewEqualMatcher("foo", "bar"))
	if seriesSet.Next() {
		t.Fatalf("Error, was expecting no matching series.")
	}
}
