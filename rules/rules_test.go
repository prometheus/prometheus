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
var ruleTests = []struct {
	rule       string
	output     []string
	shouldFail bool
}{
	{
		rule:   "SUM(http_requests)",
		output: []string{"http_requests{} => 3600 @[%v]"},
	}, {
		rule: "SUM(http_requests) BY (job)",
		output: []string{
			"http_requests{job='api-server'} => 1000 @[%v]",
			"http_requests{job='app-server'} => 2600 @[%v]",
		},
	}, {
		rule: "SUM(http_requests) BY (job, group)",
		output: []string{
			"http_requests{group='canary',job='api-server'} => 700 @[%v]",
			"http_requests{group='canary',job='app-server'} => 1500 @[%v]",
			"http_requests{group='production',job='api-server'} => 300 @[%v]",
			"http_requests{group='production',job='app-server'} => 1100 @[%v]",
		},
	}, {
		rule: "AVG(http_requests) BY (job)",
		output: []string{
			"http_requests{job='api-server'} => 250 @[%v]",
			"http_requests{job='app-server'} => 650 @[%v]",
		},
	}, {
		rule: "MIN(http_requests) BY (job)",
		output: []string{
			"http_requests{job='api-server'} => 100 @[%v]",
			"http_requests{job='app-server'} => 500 @[%v]",
		},
	}, {
		rule: "MAX(http_requests) BY (job)",
		output: []string{
			"http_requests{job='api-server'} => 400 @[%v]",
			"http_requests{job='app-server'} => 800 @[%v]",
		},
	}, {
		rule: "SUM(http_requests) BY (job) - count(http_requests)",
		output: []string{
			"http_requests{job='api-server'} => 992 @[%v]",
			"http_requests{job='app-server'} => 2592 @[%v]",
		},
	}, {
		rule: "SUM(http_requests) BY (job) - 2",
		output: []string{
			"http_requests{job='api-server'} => 998 @[%v]",
			"http_requests{job='app-server'} => 2598 @[%v]",
		},
	}, {
		rule: "SUM(http_requests) BY (job) % 3",
		output: []string{
			"http_requests{job='api-server'} => 1 @[%v]",
			"http_requests{job='app-server'} => 2 @[%v]",
		},
	}, {
		rule: "SUM(http_requests) BY (job) / 0",
		output: []string{
			"http_requests{job='api-server'} => +Inf @[%v]",
			"http_requests{job='app-server'} => +Inf @[%v]",
		},
	}, {
		rule: "SUM(http_requests) BY (job) > 1000",
		output: []string{
			"http_requests{job='app-server'} => 2600 @[%v]",
		},
	}, {
		rule: "SUM(http_requests) BY (job) <= 1000",
		output: []string{
			"http_requests{job='api-server'} => 1000 @[%v]",
		},
	}, {
		rule: "SUM(http_requests) BY (job) != 1000",
		output: []string{
			"http_requests{job='app-server'} => 2600 @[%v]",
		},
	}, {
		rule: "SUM(http_requests) BY (job) == 1000",
		output: []string{
			"http_requests{job='api-server'} => 1000 @[%v]",
		},
	}, {
		rule: "SUM(http_requests) BY (job) + SUM(http_requests) BY (job)",
		output: []string{
			"http_requests{job='api-server'} => 2000 @[%v]",
			"http_requests{job='app-server'} => 5200 @[%v]",
		},
		// Invalid rules that should fail to parse.
	}, {
		rule:       "",
		shouldFail: true,
	}, {
		rule:       "http_requests['1d']",
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

func TestRules(t *testing.T) {
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

	for _, ruleTest := range ruleTests {
		expectedLines := annotateWithTime(ruleTest.output)

		testRules, err := LoadFromString("testrule = " + ruleTest.rule)

		if err != nil {
			if ruleTest.shouldFail {
				continue
			}
			t.Errorf("Error during parsing: %v", err)
			t.Errorf("Rule: %v", ruleTest.rule)
		} else if len(testRules) != 1 {
			t.Errorf("Parser created %v rules instead of one", len(testRules))
			t.Errorf("Rule: %v", ruleTest.rule)
		} else {
			failed := false
			resultVector := testRules[0].EvalRaw(&testEvalTime)
			resultStr := resultVector.ToString()
			resultLines := strings.Split(resultStr, "\n")

			if len(ruleTest.output) != len(resultLines) {
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
				t.Errorf("Rule: %v\n%v", ruleTest.rule, vectorComparisonString(expectedLines, resultLines))
			}
		}
	}
}
