package promql
// +build gofuzz

/* PromQL parser fuzzing instrumentation for use with https://github.com/dvyukov/go-fuzz.
 * 
 * Fuzz each parser by building appropriately instrumented parser, ex. FuzzParseMetric and execute it with it's 
 *
 *     go-fuzz-build -func FuzzParseMetric -o FuzzParseMetric.zip github.com/prometheus/prometheus/promql
 *
 * And then run the tests with the appropriate inputs
 *
 *     go-fuzz -bin FuzzParseMetric.zip -workdir fuzz-data/ParseMetric
 *
 * Further input samples should go in the folders fuzz-data/ParseMetric/corpus.
 *
 * Repeat for ParseMetricSeletion, ParseExpr and ParseStmt
 */

const (
	fuzz_interesting = 1
	fuzz_meh         = 0
	fuzz_discard     = -1
)

// Fuzz the metric parser
func FuzzParseMetric(in []byte) int {
    _, err := ParseMetric(string(in))

    if err == nil {
        return fuzz_interesting
    }

    return fuzz_discard
}

// Fuzz the metric selector parser
func FuzzParseMetricSelector(in []byte) int {
    _, err := ParseMetricSelector(string(in))

    if err == nil {
        return fuzz_interesting
    }

    return fuzz_discard
}

// Fuzz the expression parser
func FuzzParseExpr(in []byte) int {
    _, err := ParseExpr(string(in))

    if err == nil {
        return fuzz_interesting
    }

    return fuzz_discard
}

// Fuzz the  parser
func FuzzParseStmts(in []byte) int {
    _, err := ParseStmts(string(in))

    if err == nil {
        return fuzz_interesting
    }

    return fuzz_discard
}
