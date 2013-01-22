package rules

import (
	"fmt"
	"github.com/matttproud/prometheus/rules/ast"
	"github.com/matttproud/prometheus/storage/metric/leveldb"
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

var testEvalTime = testStartTime.Add(testDuration5m * 10)

// Expected output needs to be alphabetically sorted (labels within one line
// must be sorted and lines between each other must be sorted too).
var expressionTests = []struct {
	expr       string
	output     []string
	shouldFail bool
}{
	{
		expr:   "SUM(http_requests)",
		output: []string{"http_requests{} => 3600 @[%v]"},
	}, {
		expr: "SUM(http_requests) BY (job)",
		output: []string{
			"http_requests{job='api-server'} => 1000 @[%v]",
			"http_requests{job='app-server'} => 2600 @[%v]",
		},
	}, {
		expr: "SUM(http_requests) BY (job, group)",
		output: []string{
			"http_requests{group='canary',job='api-server'} => 700 @[%v]",
			"http_requests{group='canary',job='app-server'} => 1500 @[%v]",
			"http_requests{group='production',job='api-server'} => 300 @[%v]",
			"http_requests{group='production',job='app-server'} => 1100 @[%v]",
		},
	}, {
		expr: "AVG(http_requests) BY (job)",
		output: []string{
			"http_requests{job='api-server'} => 250 @[%v]",
			"http_requests{job='app-server'} => 650 @[%v]",
		},
	}, {
		expr: "MIN(http_requests) BY (job)",
		output: []string{
			"http_requests{job='api-server'} => 100 @[%v]",
			"http_requests{job='app-server'} => 500 @[%v]",
		},
	}, {
		expr: "MAX(http_requests) BY (job)",
		output: []string{
			"http_requests{job='api-server'} => 400 @[%v]",
			"http_requests{job='app-server'} => 800 @[%v]",
		},
	}, {
		expr: "SUM(http_requests) BY (job) - count(http_requests)",
		output: []string{
			"http_requests{job='api-server'} => 992 @[%v]",
			"http_requests{job='app-server'} => 2592 @[%v]",
		},
	}, {
		expr: "SUM(http_requests) BY (job) - 2",
		output: []string{
			"http_requests{job='api-server'} => 998 @[%v]",
			"http_requests{job='app-server'} => 2598 @[%v]",
		},
	}, {
		expr: "SUM(http_requests) BY (job) % 3",
		output: []string{
			"http_requests{job='api-server'} => 1 @[%v]",
			"http_requests{job='app-server'} => 2 @[%v]",
		},
	}, {
		expr: "SUM(http_requests) BY (job) / 0",
		output: []string{
			"http_requests{job='api-server'} => +Inf @[%v]",
			"http_requests{job='app-server'} => +Inf @[%v]",
		},
	}, {
		expr: "SUM(http_requests) BY (job) > 1000",
		output: []string{
			"http_requests{job='app-server'} => 2600 @[%v]",
		},
	}, {
		expr: "SUM(http_requests) BY (job) <= 1000",
		output: []string{
			"http_requests{job='api-server'} => 1000 @[%v]",
		},
	}, {
		expr: "SUM(http_requests) BY (job) != 1000",
		output: []string{
			"http_requests{job='app-server'} => 2600 @[%v]",
		},
	}, {
		expr: "SUM(http_requests) BY (job) == 1000",
		output: []string{
			"http_requests{job='api-server'} => 1000 @[%v]",
		},
	}, {
		expr: "SUM(http_requests) BY (job) + SUM(http_requests) BY (job)",
		output: []string{
			"http_requests{job='api-server'} => 2000 @[%v]",
			"http_requests{job='app-server'} => 5200 @[%v]",
		},
	}, {
		expr: "delta(http_requests[25m], 1)",
		output: []string{
			"http_requests{group='canary',instance='0',job='api-server'} => 150 @[%v]",
			"http_requests{group='canary',instance='0',job='app-server'} => 350 @[%v]",
			"http_requests{group='canary',instance='1',job='api-server'} => 200 @[%v]",
			"http_requests{group='canary',instance='1',job='app-server'} => 400 @[%v]",
			"http_requests{group='production',instance='0',job='api-server'} => 50 @[%v]",
			"http_requests{group='production',instance='0',job='app-server'} => 250 @[%v]",
			"http_requests{group='production',instance='1',job='api-server'} => 100 @[%v]",
			"http_requests{group='production',instance='1',job='app-server'} => 300 @[%v]",
		},
		// Invalid expressions that should fail to parse.
	}, {
		expr:       "",
		shouldFail: true,
	}, {
		expr:       "1 - http_requests",
		shouldFail: true,
	}, {
		expr:       "http_requests['1m']",
		shouldFail: true,
	},
}

func annotateWithTime(lines []string) []string {
	annotatedLines := []string{}
	for _, line := range lines {
		annotatedLines = append(annotatedLines, fmt.Sprintf(line, testEvalTime))
	}
	return annotatedLines
}

func vectorComparisonString(expected []string, actual []string) string {
	separator := "\n--------------\n"
	return fmt.Sprintf("Expected:%v%v%v\nActual:%v%v%v ",
		separator,
		strings.Join(expected, "\n"),
		separator,
		separator,
		strings.Join(actual, "\n"),
		separator)
}

func TestExpressions(t *testing.T) {
	temporaryDirectory, err := ioutil.TempDir("", "leveldb_metric_persistence_test")
	if err != nil {
		t.Errorf("Could not create temporary directory: %q\n", err)
		return
	}
	defer func() {
		if err = os.RemoveAll(temporaryDirectory); err != nil {
			t.Errorf("Could not remove temporary directory: %q\n", err)
		}
	}()
	persistence, err := leveldb.NewLevelDBMetricPersistence(temporaryDirectory)
	if err != nil {
		t.Errorf("Could not create LevelDB Metric Persistence: %q\n", err)
		return
	}
	if persistence == nil {
		t.Errorf("Received nil LevelDB Metric Persistence.\n")
		return
	}
	defer func() {
		persistence.Close()
	}()

	storeMatrix(persistence, testMatrix)
	ast.SetPersistence(persistence)

	for _, exprTest := range expressionTests {
		expectedLines := annotateWithTime(exprTest.output)

		testExpr, err := LoadExprFromString(exprTest.expr)

		if err != nil {
			if exprTest.shouldFail {
				continue
			}
			t.Errorf("Error during parsing: %v", err)
			t.Errorf("Expression: %v", exprTest.expr)
		} else {
			failed := false
			resultStr := ast.EvalToString(testExpr, &testEvalTime, ast.TEXT)
			resultLines := strings.Split(resultStr, "\n")

			if len(exprTest.output) != len(resultLines) {
				t.Errorf("Number of samples in expected and actual output don't match")
				failed = true
			}
			for _, expectedSample := range expectedLines {
				found := false
				for _, actualSample := range resultLines {
					if actualSample == expectedSample {
						found = true
					}
				}
				if !found {
					t.Errorf("Couldn't find expected sample in output: '%v'",
						expectedSample)
					failed = true
				}
			}
			if failed {
				t.Errorf("Expression: %v\n%v", exprTest.expr, vectorComparisonString(expectedLines, resultLines))
			}
		}
	}
}
