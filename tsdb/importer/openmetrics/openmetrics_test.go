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

package openmetrics

import (
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	labels2 "github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/value"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/testutil"
)

const (
	// We use a lower value for this than the default to test block compaction implicitly.
	maxSamplesInMemory = 2000
	maxBlockChildren   = 10
)

func testBlocks(t *testing.T, blocks []tsdb.BlockReader, metricLabels []string, expectedMint, expectedMaxt int64, expectedSamples []tsdb.MetricSample, expectedSymbols []string, expectedNumBlocks int) {
	// Assert we have expected number of blocks.
	testutil.Equals(t, expectedNumBlocks, len(blocks))

	allSymbols := make(map[string]struct{})
	allSamples := make([]tsdb.MetricSample, 0)
	maxt, mint := int64(math.MinInt64), int64(math.MaxInt64)
	for _, block := range blocks {
		maxt, mint = value.MaxInt64(maxt, block.Meta().MaxTime), value.MinInt64(mint, block.Meta().MinTime)
		indexr, err := block.Index()
		testutil.Ok(t, err)
		symbols := indexr.Symbols()
		for symbols.Next() {
			key := symbols.At()
			if _, ok := allSymbols[key]; !ok {
				allSymbols[key] = struct{}{}
			}
		}
		blockSamples, err := readSeries(block, labels2.FromStrings(metricLabels...))
		testutil.Ok(t, err)
		allSamples = append(allSamples, blockSamples...)
		_ = indexr.Close()
	}

	allSymbolsSlice := make([]string, 0)
	for key := range allSymbols {
		allSymbolsSlice = append(allSymbolsSlice, key)
	}

	sort.Strings(allSymbolsSlice)
	sort.Strings(expectedSymbols)
	// Assert that all symbols that we expect to find are present.
	testutil.Equals(t, expectedSymbols, allSymbolsSlice)

	sortSamples(allSamples)
	sortSamples(expectedSamples)
	// Assert that all samples that we imported + what existed already are present.
	testutil.Assert(t, len(allSamples) == len(expectedSamples), "number of expected samples is different from actual samples")
	testutil.Equals(t, expectedSamples, allSamples, "actual samples are different from expected samples")

	// Assert that DB's time ranges are as specified.
	testutil.Equals(t, expectedMaxt, maxt)
	testutil.Equals(t, expectedMint, mint)
}

func sortSamples(samples []tsdb.MetricSample) {
	sort.Slice(samples, func(x, y int) bool {
		sx, sy := samples[x], samples[y]
		// If timestamps are equal, sort based on values.
		if sx.Timestamp != sy.Timestamp {
			return sx.Timestamp < sy.Timestamp
		}
		return sx.Value < sy.Value
	})
}

// readSeries returns all series present in the block, that contain the labels we want.
// The labels do not have to be exhaustive, i.e. if we have metrics with common labels b/w them,
// we just need to pass the common labels, and we will get all the metrics that have them,
// even if the other labels b/w the metrics are different.
func readSeries(block tsdb.BlockReader, lbls labels2.Labels) ([]tsdb.MetricSample, error) {
	series := make([]tsdb.MetricSample, 0)
	ir, err := block.Index()
	if err != nil {
		return series, err
	}
	defer ir.Close()
	tsr, err := block.Tombstones()
	if err != nil {
		return series, err
	}
	defer tsr.Close()
	chunkr, err := block.Chunks()
	if err != nil {
		return series, err
	}
	defer chunkr.Close()
	for _, lbl := range lbls {
		css, err := tsdb.LookupChunkSeries(ir, tsr, labels2.MustNewMatcher(labels2.MatchEqual, lbl.Name, lbl.Value))
		if err != nil {
			return series, err
		}
		for css.Next() {
			actLabels, chkMetas, _ := css.At()
			var chunkIter chunkenc.Iterator
			for _, meta := range chkMetas {
				chk, err := chunkr.Chunk(meta.Ref)
				if err != nil {
					return series, err
				}
				chunkIter = chk.Iterator(chunkIter)
				for chunkIter.Next() {
					t, v := chunkIter.At()
					sample := tsdb.MetricSample{
						Timestamp: t,
						Value:     v,
						Labels:    actLabels,
					}
					series = append(series, sample)
				}
			}
		}
	}
	return series, nil
}

// genSeries generates series from mint to maxt, with a step value.
func genSeries(labels []string, mint, maxt int64, step int) []tsdb.MetricSample {
	series := make([]tsdb.MetricSample, 0)
	for idx := mint; idx < maxt; idx += int64(step) {
		// Round to 3 places.
		val := math.Floor(rand.Float64()*1000) / 1000
		sample := tsdb.MetricSample{Timestamp: idx, Value: val, Labels: labels2.FromStrings(labels...)}
		series = append(series, sample)
	}
	return series
}

// labelsToStr converts the given labels to a string representation that is compliant with
// the OpenMetrics parser.
func labelsToStr(labels labels2.Labels) string {
	str := "{"
	for idx, l := range labels {
		str += fmt.Sprintf("%s=%s", l.Name, strconv.Quote(l.Value))
		if idx < len(labels)-1 {
			str += ","
		}
	}
	str += "}"
	return str
}

// genOpenMetricsText formats the given series data into OpenMetrics compliant format.
func genOpenMetricsText(metricName, metricType string, series []tsdb.MetricSample) string {
	str := fmt.Sprintf("# HELP %s This is a metric\n# TYPE %s %s", metricName, metricName, metricType)
	for _, s := range series {
		str += fmt.Sprintf("\n%s%s %f %d", metricName, labelsToStr(s.Labels), s.Value, s.Timestamp)
	}
	str += fmt.Sprintf("\n# EOF")
	return str
}

func TestImport(t *testing.T) {
	tests := []struct {
		ToParse      string
		IsOk         bool
		MetricLabels []string
		Expected     struct {
			MinTime   int64
			MaxTime   int64
			NumBlocks int
			Symbols   []string
			Samples   []tsdb.MetricSample
		}
	}{
		{
			ToParse: `# EOF`,
			IsOk:    true,
		},
		{
			ToParse: `# HELP http_requests_total The total number of HTTP requests.
# TYPE http_requests_total counter
http_requests_total{code="200"} 1021 1565133713989
http_requests_total{code="400"} 1 1565133713990
# EOF
`,
			IsOk:         true,
			MetricLabels: []string{"__name__", "http_requests_total"},
			Expected: struct {
				MinTime   int64
				MaxTime   int64
				NumBlocks int
				Symbols   []string
				Samples   []tsdb.MetricSample
			}{
				MinTime:   1565133713989,
				MaxTime:   1565133713991,
				NumBlocks: 1,
				Symbols:   []string{"__name__", "http_requests_total", "code", "200", "400"},
				Samples: []tsdb.MetricSample{
					{
						Timestamp: 1565133713989,
						Value:     1021,
						Labels:    labels2.FromStrings("__name__", "http_requests_total", "code", "200"),
					},
					{
						Timestamp: 1565133713990,
						Value:     1,
						Labels:    labels2.FromStrings("__name__", "http_requests_total", "code", "400"),
					},
				},
			},
		},
	}
	for _, test := range tests {
		tmpDbDir, err := ioutil.TempDir("", "importer")
		testutil.Ok(t, err)
		err = ImportFromFile(strings.NewReader(test.ToParse), tmpDbDir, maxSamplesInMemory, maxBlockChildren, nil)
		if test.IsOk {
			testutil.Ok(t, err)
			if len(test.Expected.Symbols) > 0 {
				db, err := tsdb.OpenDBReadOnly(tmpDbDir, nil)
				testutil.Ok(t, err)
				blocks, err := db.Blocks()
				testutil.Ok(t, err)
				testBlocks(t, blocks, test.MetricLabels, test.Expected.MinTime, test.Expected.MaxTime, test.Expected.Samples, test.Expected.Symbols, test.Expected.NumBlocks)
			}
		} else {
			testutil.NotOk(t, err)
		}
		_ = os.RemoveAll(tmpDbDir)
	}
}

func TestImportBadFile(t *testing.T) {
	// No file found case.
	err := ImportFromFile((*os.File)(nil), "/buzz/baz/bar/foo", maxSamplesInMemory, maxBlockChildren, nil)
	testutil.NotOk(t, err)
}

func TestImportIntoExistingDB(t *testing.T) {
	type importTest struct {
		MetricName                 string
		MetricType                 string
		MetricLabels               []string
		GeneratorStep              int
		DBMint, DBMaxt             int64
		ImportShuffle              bool
		ImportMint, ImportMaxt     int64
		ExpectedMint, ExpectedMaxt int64
		ExpectedSymbols            []string
		ExpectedNumBlocks          int
	}

	tests := []importTest{
		{
			// Import overlapping data only.
			MetricName:        "test_metric",
			MetricType:        "gauge",
			MetricLabels:      []string{"foo", "bar"},
			GeneratorStep:     1,
			DBMint:            1000,
			DBMaxt:            2000,
			ImportShuffle:     false,
			ImportMint:        1000,
			ImportMaxt:        1500,
			ExpectedMint:      1000,
			ExpectedMaxt:      2000,
			ExpectedSymbols:   []string{"__name__", "test_metric", "foo", "bar"},
			ExpectedNumBlocks: 2,
		},
		{
			// Test overlapping, and non-overlapping data, with a lot of samples.
			MetricName:        "test_metric_2",
			MetricType:        "gauge",
			MetricLabels:      []string{"foo", "bar"},
			GeneratorStep:     1,
			DBMint:            1000,
			DBMaxt:            4000,
			ImportShuffle:     false,
			ImportMint:        0,
			ImportMaxt:        5000,
			ExpectedSymbols:   []string{"__name__", "test_metric_2", "foo", "bar"},
			ExpectedMint:      0,
			ExpectedMaxt:      5000,
			ExpectedNumBlocks: 4,
		},
		{
			// Test overlapping, and non-overlapping data, aligning the blocks.
			// This test also creates a large number of samples, across a much wider time range, to test
			// if the importer will divvy samples beyond DB time limits.
			MetricName:        "test_metric_3",
			MetricType:        "gauge",
			MetricLabels:      []string{"foo", "bar"},
			GeneratorStep:     20000,
			DBMint:            1000,
			DBMaxt:            tsdb.DefaultBlockDuration,
			ImportShuffle:     false,
			ImportMint:        0,
			ImportMaxt:        tsdb.DefaultBlockDuration * 2,
			ExpectedSymbols:   []string{"__name__", "test_metric_3", "foo", "bar"},
			ExpectedMint:      0,
			ExpectedMaxt:      (tsdb.DefaultBlockDuration * 2) - 20000 + 1,
			ExpectedNumBlocks: 4,
		},
		{
			// Import partially overlapping data only.
			MetricName:        "test_metric_4",
			MetricType:        "gauge",
			MetricLabels:      []string{"foo", "bar"},
			GeneratorStep:     1,
			DBMint:            1000,
			DBMaxt:            2000,
			ImportShuffle:     false,
			ImportMint:        500,
			ImportMaxt:        1500,
			ExpectedMint:      500,
			ExpectedMaxt:      2000,
			ExpectedSymbols:   []string{"__name__", "test_metric_4", "foo", "bar"},
			ExpectedNumBlocks: 3,
		},
		{
			// Import that never overlaps, after the DB max time.
			MetricName:        "test_metric_5",
			MetricType:        "gauge",
			MetricLabels:      []string{"foo", "bar"},
			GeneratorStep:     1,
			DBMint:            0,
			DBMaxt:            1000,
			ImportShuffle:     false,
			ImportMint:        tsdb.DefaultBlockDuration + 1000,
			ImportMaxt:        tsdb.DefaultBlockDuration + 2000,
			ExpectedMint:      0,
			ExpectedMaxt:      tsdb.DefaultBlockDuration + 2000,
			ExpectedSymbols:   []string{"__name__", "test_metric_5", "foo", "bar"},
			ExpectedNumBlocks: 2,
		},
	}

	for _, test := range tests {
		initSeries := genSeries(test.MetricLabels, test.DBMint, test.DBMaxt, test.GeneratorStep)
		initText := genOpenMetricsText(test.MetricName, test.MetricType, initSeries)

		tmpDbDir, err := ioutil.TempDir("", "importer")
		testutil.Ok(t, err)
		err = ImportFromFile(strings.NewReader(initText), tmpDbDir, maxSamplesInMemory, maxBlockChildren, nil)
		testutil.Ok(t, err)

		importSeries := genSeries(test.MetricLabels, test.ImportMint, test.ImportMaxt, test.GeneratorStep)
		importText := genOpenMetricsText(test.MetricName, test.MetricType, importSeries)

		err = ImportFromFile(strings.NewReader(importText), tmpDbDir, maxSamplesInMemory, maxBlockChildren, nil)
		testutil.Ok(t, err)

		expectedSamples := make([]tsdb.MetricSample, 0)
		for _, exp := range [][]tsdb.MetricSample{initSeries, importSeries} {
			for _, sample := range exp {
				lbls := labels2.FromStrings("__name__", test.MetricName)
				s := tsdb.MetricSample{
					Timestamp: sample.Timestamp,
					Value:     sample.Value,
					Labels:    append(lbls, sample.Labels...),
				}
				expectedSamples = append(expectedSamples, s)
			}
		}

		db, err := tsdb.Open(tmpDbDir, nil, nil, &tsdb.Options{
			RetentionDuration:      tsdb.DefaultOptions().RetentionDuration,
			AllowOverlappingBlocks: true,
		})
		testutil.Ok(t, err)

		blocks := make([]tsdb.BlockReader, 0)
		for _, b := range db.Blocks() {
			blocks = append(blocks, b)
		}
		testBlocks(t, blocks, test.MetricLabels, test.ExpectedMint, test.ExpectedMaxt, expectedSamples, test.ExpectedSymbols, test.ExpectedNumBlocks)

		// Close and remove all temp files and folders.
		_ = db.Close()
		_ = os.RemoveAll(tmpDbDir)
	}
}

// Test to see if multiple series are imported correctly.
func TestMixedSeries(t *testing.T) {
	partialOMText := func(metricName, metricType string, series []tsdb.MetricSample) string {
		str := fmt.Sprintf("# HELP %s This is a metric\n# TYPE %s %s", metricName, metricName, metricType)
		for _, s := range series {
			str += fmt.Sprintf("\n%s%s %f %d", metricName, s.Labels.String(), s.Value, s.Timestamp)
		}
		return str
	}
	metricA := "metricA"
	metricLabels := []string{"foo", "bar"}
	mint, maxt := int64(0), int64(1000)
	step := 5
	seriesA := genSeries(metricLabels, mint, maxt, step)
	textA := partialOMText(metricA, "gauge", seriesA)
	metricB := "metricB"
	seriesB := genSeries(metricLabels, mint, maxt, step)
	textB := partialOMText(metricB, "gauge", seriesB)

	text := fmt.Sprintf("%s\n%s\n# EOF", textA, textB)

	tmpDbDir, err := ioutil.TempDir("", "importer")
	testutil.Ok(t, err)
	err = ImportFromFile(strings.NewReader(text), tmpDbDir, maxSamplesInMemory, maxBlockChildren, nil)
	testutil.Ok(t, err)

	addMetricLabel := func(series []tsdb.MetricSample, metricName string) []tsdb.MetricSample {
		augSamples := make([]tsdb.MetricSample, 0)
		for _, sample := range series {
			lbls := labels2.FromStrings("__name__", metricName)
			s := tsdb.MetricSample{
				Timestamp: sample.Timestamp,
				Value:     sample.Value,
				Labels:    append(lbls, sample.Labels...),
			}
			augSamples = append(augSamples, s)
		}
		return augSamples
	}

	expectedSamples := append(addMetricLabel(seriesA, metricA), addMetricLabel(seriesB, metricB)...)
	expectedSymbols := append([]string{"__name__", metricA, metricB}, metricLabels...)

	db, err := tsdb.Open(tmpDbDir, nil, nil, &tsdb.Options{
		RetentionDuration:      tsdb.DefaultOptions().RetentionDuration,
		AllowOverlappingBlocks: true,
	})
	testutil.Ok(t, err)

	blocks := make([]tsdb.BlockReader, 0)
	for _, b := range db.Blocks() {
		blocks = append(blocks, b)
	}

	testBlocks(t, blocks, metricLabels, mint, maxt-int64(step-1), expectedSamples, expectedSymbols, 1)

	_ = os.RemoveAll(tmpDbDir)
}

// Test to check if we can detect an error with the text,
// without having to read till EOF.
func TestInvalidSyntaxQuickAbort(t *testing.T) {
	lbls := []string{"foo", "bar"}
	firstHalf := genSeries(lbls, 0, 10, 1)
	invalidMetrics := []tsdb.MetricSample{
		{
			Timestamp: -1,
			Value:     0,
			Labels:    labels2.FromStrings(" INVALID", "INVALID"),
		},
	}
	secondHalf := genSeries(lbls, 10, 200, 1)
	combined := append(firstHalf, append(invalidMetrics, secondHalf...)...)
	importText := genOpenMetricsText("invalid_test", "gauge", combined)

	tmpDbDir, err := ioutil.TempDir("", "importer")
	testutil.Ok(t, err)

	r := strings.NewReader(importText)
	err = ImportFromFile(r, tmpDbDir, maxSamplesInMemory, maxBlockChildren, nil)

	testutil.NotOk(t, err)
	// We should abort and return the error with the text, without reading till EOF.
	testutil.Assert(t, r.Len() > 0, "import text has been completely read")
}
