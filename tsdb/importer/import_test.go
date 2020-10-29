package importer

import (
	"os"
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
)

// import (
// 	"io/ioutil"
// 	"os"
// 	"strings"
// 	"testing"

// 	"github.com/prometheus/prometheus/tsdb"
// 	"github.com/prometheus/prometheus/util/testutil"
// )

func TestCleanDir(t *testing.T) {
	t.Cleanup(func() {
		testutil.Ok(t, os.RemoveAll("/Users/atibhi/Desktop/prometheus/tmpDir"))
	})
}

// func TestImport(t *testing.T) {
// 	tests := []struct {
// 		ToParse      string
// 		IsOk         bool
// 		MetricLabels []string
// 		Expected: struct {
// 			MinTime   int64
// 			MaxTime   int64
// 			NumBlocks int
// 			Symbols   []string
// 			// Samples   []tsdb.MetricSample
// 		}
// 	}{
// 		{
// 			ToParse: `# EOF`,
// 			IsOk:    true,
// 		},
// 		{
// 			ToParse: `# HELP http_requests_total The total number of HTTP requests.
// # TYPE http_requests_total counter
// http_requests_total{code="200"} 1021 1565133713989
// http_requests_total{code="400"} 1 1565133713990
// # EOF
// `,
// 			IsOk:         true,
// 			MetricLabels: []string{"__name__", "http_requests_total"},
// 			Expected: struct {
// 				MinTime   int64
// 				MaxTime   int64
// 				NumBlocks int
// 				Symbols   []string
// 				// Samples   []tsdb.MetricSample
// 			}{
// 				MinTime:   1565133713989,
// 				MaxTime:   1565133713991,
// 				NumBlocks: 1,
// 				Symbols:   []string{"__name__", "http_requests_total", "code", "200", "400"},
// 				// Samples:   []tsdb.MetricSample{
// 				// {
// 				// 	Timestamp: 1565133713989,
// 				// 	Value:     1021,
// 				// 	Labels:    labels2.FromStrings("__name__", "http_requests_total", "code", "200"),
// 				// },
// 				// {
// 				// 	Timestamp: 1565133713990,
// 				// 	Value:     1,
// 				// 	Labels:    labels2.FromStrings("__name__", "http_requests_total", "code", "400"),
// 				// },
// 				//},
// 			},
// 		},
// 	}
// 	for _, test := range tests {
// 		tmpDbDir, err := ioutil.TempDir("", "importer")
// 		testutil.Ok(t, err)
// 		err = ImportFromFile(strings.NewReader(test.ToParse), tmpDbDir, maxSamplesInMemory, maxBlockChildren, nil)
// 		if test.IsOk {
// 			testutil.Ok(t, err)
// 			if len(test.Expected.Symbols) > 0 {
// 				db, err := tsdb.OpenDBReadOnly(tmpDbDir, nil)
// 				testutil.Ok(t, err)
// 				blocks, err := db.Blocks()
// 				testutil.Ok(t, err)
// 				testBlocks(t, blocks, test.MetricLabels, test.Expected.MinTime, test.Expected.MaxTime, test.Expected.Samples, test.Expected.Symbols, test.Expected.NumBlocks)
// 			}
// 		} else {
// 			testutil.NotOk(t, err)
// 		}
// 		_ = os.RemoveAll(tmpDbDir)
// 	}
// }
