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

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"testing"
	"text/tabwriter"
	"time"

	"github.com/prometheus/prometheus/tsdb"
)

func createRoDb(t *testing.T) (*tsdb.DBReadOnly, func()) {
	tmpdir, err := ioutil.TempDir("", "test")
	if err != nil {
		os.RemoveAll(tmpdir)
		t.Error(err)
	}

	safeDBOptions := *tsdb.DefaultOptions
	safeDBOptions.RetentionDuration = 0

	tsdb.CreateBlock(nil, tmpdir, tsdb.GenSeries(1, 1, 0, 1))

	dbRO, err := tsdb.OpenDBReadOnly(tmpdir, nil)
	if err != nil {
		t.Error(err)
	}

	return dbRO, func() {
		os.RemoveAll(tmpdir)
	}
}

func TestPrintBlocks(t *testing.T) {
	db, closeFn := createRoDb(t)
	defer closeFn()

	var b bytes.Buffer
	hr := false
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	defer tw.Flush()

	// Set table header.
	_, err := fmt.Fprintln(&b, printBlocksTableHeader)
	if err != nil {
		t.Error(err)
	}

	// Test table header.
	actual := b.String()
	expected := fmt.Sprintln(printBlocksTableHeader)
	if expected != actual {
		t.Errorf("expected (%#v) != actual (%#v)", expected, actual)
	}

	// Set table contents.
	blocks, err := db.Blocks()
	if err != nil {
		t.Error(err)
	}
	meta := blocks[0].Meta()

	_, err = fmt.Fprintf(&b,
		"%v\t%v\t%v\t%v\t%v\t%v\n",
		meta.ULID,
		getFormatedTime(meta.MinTime, &hr),
		getFormatedTime(meta.MaxTime, &hr),
		meta.Stats.NumSamples,
		meta.Stats.NumChunks,
		meta.Stats.NumSeries,
	)

	if err != nil {
		t.Error(err)
	}

	// Test table contents.
	blocks, err = db.Blocks()
	if err != nil {
		t.Error(err)
	}

	var actualStdout bytes.Buffer
	printBlocks(&actualStdout, blocks, &hr)

	actual = actualStdout.String()
	actual = strings.Replace(actual, " ", "", -1)
	actual = strings.Replace(actual, "\t", "", -1)
	actual = strings.Replace(actual, "\n", "", -1)

	expected = b.String()
	expected = strings.Replace(expected, " ", "", -1)
	expected = strings.Replace(expected, "\t", "", -1)
	expected = strings.Replace(expected, "\n", "", -1)

	if expected != actual {
		t.Errorf("expected (%#v) != actual (%#v)", b.String(), actualStdout.String())
	}
}

func TestExtractBlock(t *testing.T) {
	db, closeFn := createRoDb(t)
	defer closeFn()

	blocks, err := db.Blocks()
	if err != nil {
		t.Error(err)
	}

	var analyzeBlockID string

	// Pass: analyze last block (default).
	block, err := extractBlock(blocks, &analyzeBlockID)
	if err != nil {
		t.Error(err)
	}
	if block == nil {
		t.Error("block shouldn't be nil")
	}

	// Pass: analyze specific block.
	analyzeBlockID = block.Meta().ULID.String()
	block, err = extractBlock(blocks, &analyzeBlockID)
	if err != nil {
		t.Error(err)
	}
	if block == nil {
		t.Error("block shouldn't be nil")
	}

	// Fail: analyze non-existing block
	analyzeBlockID = "foo"
	block, err = extractBlock(blocks, &analyzeBlockID)
	if err == nil {
		t.Errorf("Analyzing block ID %q should throw error", analyzeBlockID)
	}
	if block != nil {
		t.Error("block should be nil")
	}
}

func TestAnalyzeBlocks(t *testing.T) {
	db, closeFn := createRoDb(t)
	defer closeFn()

	blocks, err := db.Blocks()
	if err != nil {
		t.Error(err)
	}

	var analyzeBlockID string
	block, err := extractBlock(blocks, &analyzeBlockID)
	if err != nil {
		t.Error(err)
	}
	if block == nil {
		t.Errorf("block shouldn't be nil")
	}

	dal, err := strconv.Atoi(defaultAnalyzeLimit)
	if err != nil {
		t.Error(err)
	}

	var (
		expected bytes.Buffer
		actual   bytes.Buffer
	)

	// Actual output.
	err = analyzeBlock(&actual, block, dal)
	if err != nil {
		t.Error(err)
	}

	act := actual.String()
	act = strings.Replace(act, " ", "", -1)
	act = strings.Replace(act, "\t", "", -1)
	act = strings.Replace(act, "\n", "", -1)

	// Expected output.
	meta := block.Meta()
	fmt.Fprintf(&expected, "Block ID: %s\n", meta.ULID)
	fmt.Fprintf(&expected, "Duration: %s\n", (time.Duration(meta.MaxTime-meta.MinTime) * 1e6).String())
	fmt.Fprintf(&expected, "Series: %d\n", 1)
	fmt.Fprintf(&expected, "Label names: %d\n", 1)
	fmt.Fprintf(&expected, "Postings (unique label pairs): %d\n", 1)
	fmt.Fprintf(&expected, "Postings entries (total label pairs): %d\n", 1)
	fmt.Fprintf(&expected, "\nLabel pairs most involved in churning:\n")
	fmt.Fprintf(&expected, "1 %s=0", tsdb.MockDefaultLabelName)
	fmt.Fprintf(&expected, "\nLabel names most involved in churning:\n")
	fmt.Fprintf(&expected, "1 %s", tsdb.MockDefaultLabelName)
	fmt.Fprintf(&expected, "\nMost common label pairs:\n")
	fmt.Fprintf(&expected, "1 %s=0", tsdb.MockDefaultLabelName)
	fmt.Fprintf(&expected, "\nLabel names with highest cumulative label value length:\n")
	fmt.Fprintf(&expected, "1 %s", tsdb.MockDefaultLabelName)
	fmt.Fprintf(&expected, "\nHighest cardinality labels:\n")
	fmt.Fprintf(&expected, "1 %s", tsdb.MockDefaultLabelName)
	fmt.Fprintf(&expected, "\nHighest cardinality metric names:\n")

	exp := expected.String()
	exp = strings.Replace(exp, " ", "", -1)
	exp = strings.Replace(exp, "\t", "", -1)
	exp = strings.Replace(exp, "\n", "", -1)

	if exp != act {
		t.Errorf("expected (%#v) != actual (%#v)", expected.String(), actual.String())
	}
}
