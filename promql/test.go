// Copyright 2015 The Prometheus Authors
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

package promql

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grafana/regexp"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
	"github.com/prometheus/prometheus/util/testutil"
)

var (
	minNormal = math.Float64frombits(0x0010000000000000) // The smallest positive normal value of type float64.

	patSpace       = regexp.MustCompile("[\t ]+")
	patLoad        = regexp.MustCompile(`^load\s+(.+?)$`)
	patEvalInstant = regexp.MustCompile(`^eval(?:_(fail|ordered))?\s+instant\s+(?:at\s+(.+?))?\s+(.+)$`)
	patEvalRange   = regexp.MustCompile(`^eval(?:_(fail))?\s+range\s+from\s+(.+)\s+to\s+(.+)\s+step\s+(.+?)\s+(.+)$`)
)

const (
	defaultEpsilon = 0.000001 // Relative error allowed for sample values.
)

var testStartTime = time.Unix(0, 0).UTC()

// LoadedStorage returns storage with generated data using the provided load statements.
// Non-load statements will cause test errors.
func LoadedStorage(t testutil.T, input string) *teststorage.TestStorage {
	test, err := newTest(t, input)
	require.NoError(t, err)

	for _, cmd := range test.cmds {
		switch cmd.(type) {
		case *loadCmd:
			require.NoError(t, test.exec(cmd, nil))
		default:
			t.Errorf("only 'load' commands accepted, got '%s'", cmd)
		}
	}
	return test.storage
}

// RunBuiltinTests runs an acceptance test suite against the provided engine.
func RunBuiltinTests(t *testing.T, engine QueryEngine) {
	t.Cleanup(func() { parser.EnableExperimentalFunctions = false })
	parser.EnableExperimentalFunctions = true

	files, err := fs.Glob(testsFs, "*/*.test")
	require.NoError(t, err)

	for _, fn := range files {
		t.Run(fn, func(t *testing.T) {
			content, err := fs.ReadFile(testsFs, fn)
			require.NoError(t, err)
			RunTest(t, string(content), engine)
		})
	}
}

// RunTest parses and runs the test against the provided engine.
func RunTest(t testutil.T, input string, engine QueryEngine) {
	require.NoError(t, runTest(t, input, engine))
}

func runTest(t testutil.T, input string, engine QueryEngine) error {
	test, err := newTest(t, input)

	// Why do this before checking err? newTest() can create the test storage and then return an error,
	// and we want to make sure to clean that up to avoid leaking goroutines.
	defer func() {
		if test == nil {
			return
		}
		if test.storage != nil {
			test.storage.Close()
		}
		if test.cancelCtx != nil {
			test.cancelCtx()
		}
	}()

	if err != nil {
		return err
	}

	for _, cmd := range test.cmds {
		if err := test.exec(cmd, engine); err != nil {
			// TODO(fabxc): aggregate command errors, yield diffs for result
			// comparison errors.
			return err
		}
	}

	return nil
}

// test is a sequence of read and write commands that are run
// against a test storage.
type test struct {
	testutil.T

	cmds []testCommand

	storage *teststorage.TestStorage

	context   context.Context
	cancelCtx context.CancelFunc
}

// newTest returns an initialized empty Test.
func newTest(t testutil.T, input string) (*test, error) {
	test := &test{
		T:    t,
		cmds: []testCommand{},
	}
	err := test.parse(input)
	test.clear()

	return test, err
}

//go:embed testdata
var testsFs embed.FS

func raise(line int, format string, v ...interface{}) error {
	return &parser.ParseErr{
		LineOffset: line,
		Err:        fmt.Errorf(format, v...),
	}
}

func parseLoad(lines []string, i int) (int, *loadCmd, error) {
	if !patLoad.MatchString(lines[i]) {
		return i, nil, raise(i, "invalid load command. (load <step:duration>)")
	}
	parts := patLoad.FindStringSubmatch(lines[i])

	gap, err := model.ParseDuration(parts[1])
	if err != nil {
		return i, nil, raise(i, "invalid step definition %q: %s", parts[1], err)
	}
	cmd := newLoadCmd(time.Duration(gap))
	for i+1 < len(lines) {
		i++
		defLine := lines[i]
		if len(defLine) == 0 {
			i--
			break
		}
		metric, vals, err := parseSeries(defLine, i)
		if err != nil {
			return i, nil, err
		}
		cmd.set(metric, vals...)
	}
	return i, cmd, nil
}

func parseSeries(defLine string, line int) (labels.Labels, []parser.SequenceValue, error) {
	metric, vals, err := parser.ParseSeriesDesc(defLine)
	if err != nil {
		parser.EnrichParseError(err, func(parseErr *parser.ParseErr) {
			parseErr.LineOffset = line
		})
		return labels.Labels{}, nil, err
	}
	return metric, vals, nil
}

func (t *test) parseEval(lines []string, i int) (int, *evalCmd, error) {
	instantParts := patEvalInstant.FindStringSubmatch(lines[i])
	rangeParts := patEvalRange.FindStringSubmatch(lines[i])

	if instantParts == nil && rangeParts == nil {
		return i, nil, raise(i, "invalid evaluation command. Must be either 'eval[_fail|_ordered] instant [at <offset:duration>] <query>' or 'eval[_fail] range from <from> to <to> step <step> <query>'")
	}

	isInstant := instantParts != nil

	var mod string
	var expr string

	if isInstant {
		mod = instantParts[1]
		expr = instantParts[3]
	} else {
		mod = rangeParts[1]
		expr = rangeParts[5]
	}

	_, err := parser.ParseExpr(expr)
	if err != nil {
		parser.EnrichParseError(err, func(parseErr *parser.ParseErr) {
			parseErr.LineOffset = i
			posOffset := posrange.Pos(strings.Index(lines[i], expr))
			parseErr.PositionRange.Start += posOffset
			parseErr.PositionRange.End += posOffset
			parseErr.Query = lines[i]
		})
		return i, nil, err
	}

	formatErr := func(format string, args ...any) error {
		combinedArgs := []any{expr, i + 1}

		combinedArgs = append(combinedArgs, args...)
		return fmt.Errorf("error in eval %s (line %v): "+format, combinedArgs...)
	}

	var cmd *evalCmd

	if isInstant {
		at := instantParts[2]
		offset, err := model.ParseDuration(at)
		if err != nil {
			return i, nil, formatErr("invalid timestamp definition %q: %s", at, err)
		}
		ts := testStartTime.Add(time.Duration(offset))
		cmd = newInstantEvalCmd(expr, ts, i+1)
	} else {
		from := rangeParts[2]
		to := rangeParts[3]
		step := rangeParts[4]

		parsedFrom, err := model.ParseDuration(from)
		if err != nil {
			return i, nil, formatErr("invalid start timestamp definition %q: %s", from, err)
		}

		parsedTo, err := model.ParseDuration(to)
		if err != nil {
			return i, nil, formatErr("invalid end timestamp definition %q: %s", to, err)
		}

		if parsedTo < parsedFrom {
			return i, nil, formatErr("invalid test definition, end timestamp (%s) is before start timestamp (%s)", to, from)
		}

		parsedStep, err := model.ParseDuration(step)
		if err != nil {
			return i, nil, formatErr("invalid step definition %q: %s", step, err)
		}

		cmd = newRangeEvalCmd(expr, testStartTime.Add(time.Duration(parsedFrom)), testStartTime.Add(time.Duration(parsedTo)), time.Duration(parsedStep), i+1)
	}

	switch mod {
	case "ordered":
		// Ordered results are not supported for range queries, but the regex for range query commands does not allow
		// asserting an ordered result, so we don't need to do any error checking here.
		cmd.ordered = true
	case "fail":
		cmd.fail = true
	}

	for j := 1; i+1 < len(lines); j++ {
		i++
		defLine := lines[i]
		if len(defLine) == 0 {
			i--
			break
		}
		if f, err := parseNumber(defLine); err == nil {
			cmd.expect(0, parser.SequenceValue{Value: f})
			break
		}
		metric, vals, err := parseSeries(defLine, i)
		if err != nil {
			return i, nil, err
		}

		// Currently, we are not expecting any matrices.
		if len(vals) > 1 && isInstant {
			return i, nil, formatErr("expecting multiple values in instant evaluation not allowed")
		}
		cmd.expectMetric(j, metric, vals...)
	}
	return i, cmd, nil
}

// getLines returns trimmed lines after removing the comments.
func getLines(input string) []string {
	lines := strings.Split(input, "\n")
	for i, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "#") {
			l = ""
		}
		lines[i] = l
	}
	return lines
}

// parse the given command sequence and appends it to the test.
func (t *test) parse(input string) error {
	lines := getLines(input)
	var err error
	// Scan for steps line by line.
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if len(l) == 0 {
			continue
		}
		var cmd testCommand

		switch c := strings.ToLower(patSpace.Split(l, 2)[0]); {
		case c == "clear":
			cmd = &clearCmd{}
		case c == "load":
			i, cmd, err = parseLoad(lines, i)
		case strings.HasPrefix(c, "eval"):
			i, cmd, err = t.parseEval(lines, i)
		default:
			return raise(i, "invalid command %q", l)
		}
		if err != nil {
			return err
		}
		t.cmds = append(t.cmds, cmd)
	}
	return nil
}

// testCommand is an interface that ensures that only the package internal
// types can be a valid command for a test.
type testCommand interface {
	testCmd()
}

func (*clearCmd) testCmd() {}
func (*loadCmd) testCmd()  {}
func (*evalCmd) testCmd()  {}

// loadCmd is a command that loads sequences of sample values for specific
// metrics into the storage.
type loadCmd struct {
	gap       time.Duration
	metrics   map[uint64]labels.Labels
	defs      map[uint64][]Sample
	exemplars map[uint64][]exemplar.Exemplar
}

func newLoadCmd(gap time.Duration) *loadCmd {
	return &loadCmd{
		gap:       gap,
		metrics:   map[uint64]labels.Labels{},
		defs:      map[uint64][]Sample{},
		exemplars: map[uint64][]exemplar.Exemplar{},
	}
}

func (cmd loadCmd) String() string {
	return "load"
}

// set a sequence of sample values for the given metric.
func (cmd *loadCmd) set(m labels.Labels, vals ...parser.SequenceValue) {
	h := m.Hash()

	samples := make([]Sample, 0, len(vals))
	ts := testStartTime
	for _, v := range vals {
		if !v.Omitted {
			samples = append(samples, Sample{
				T: ts.UnixNano() / int64(time.Millisecond/time.Nanosecond),
				F: v.Value,
				H: v.Histogram,
			})
		}
		ts = ts.Add(cmd.gap)
	}
	cmd.defs[h] = samples
	cmd.metrics[h] = m
}

// append the defined time series to the storage.
func (cmd *loadCmd) append(a storage.Appender) error {
	for h, smpls := range cmd.defs {
		m := cmd.metrics[h]

		for _, s := range smpls {
			if err := appendSample(a, s, m); err != nil {
				return err
			}
		}
	}
	return nil
}

func appendSample(a storage.Appender, s Sample, m labels.Labels) error {
	if s.H != nil {
		if _, err := a.AppendHistogram(0, m, s.T, nil, s.H); err != nil {
			return err
		}
	} else {
		if _, err := a.Append(0, m, s.T, s.F); err != nil {
			return err
		}
	}
	return nil
}

// evalCmd is a command that evaluates an expression for the given time (range)
// and expects a specific result.
type evalCmd struct {
	expr  string
	start time.Time
	end   time.Time
	step  time.Duration
	line  int

	isRange       bool // if false, instant query
	fail, ordered bool

	metrics  map[uint64]labels.Labels
	expected map[uint64]entry
}

type entry struct {
	pos  int
	vals []parser.SequenceValue
}

func (e entry) String() string {
	return fmt.Sprintf("%d: %s", e.pos, e.vals)
}

func newInstantEvalCmd(expr string, start time.Time, line int) *evalCmd {
	return &evalCmd{
		expr:  expr,
		start: start,
		line:  line,

		metrics:  map[uint64]labels.Labels{},
		expected: map[uint64]entry{},
	}
}

func newRangeEvalCmd(expr string, start, end time.Time, step time.Duration, line int) *evalCmd {
	return &evalCmd{
		expr:    expr,
		start:   start,
		end:     end,
		step:    step,
		line:    line,
		isRange: true,

		metrics:  map[uint64]labels.Labels{},
		expected: map[uint64]entry{},
	}
}

func (ev *evalCmd) String() string {
	return "eval"
}

// expect adds a sequence of values to the set of expected
// results for the query.
func (ev *evalCmd) expect(pos int, vals ...parser.SequenceValue) {
	ev.expected[0] = entry{pos: pos, vals: vals}
}

// expectMetric adds a new metric with a sequence of values to the set of expected
// results for the query.
func (ev *evalCmd) expectMetric(pos int, m labels.Labels, vals ...parser.SequenceValue) {
	h := m.Hash()
	ev.metrics[h] = m
	ev.expected[h] = entry{pos: pos, vals: vals}
}

// compareResult compares the result value with the defined expectation.
func (ev *evalCmd) compareResult(result parser.Value) error {
	switch val := result.(type) {
	case Matrix:
		if ev.ordered {
			return fmt.Errorf("expected ordered result, but query returned a matrix")
		}

		if err := assertMatrixSorted(val); err != nil {
			return err
		}

		seen := map[uint64]bool{}
		for _, s := range val {
			hash := s.Metric.Hash()
			if _, ok := ev.metrics[hash]; !ok {
				return fmt.Errorf("unexpected metric %s in result", s.Metric)
			}
			seen[hash] = true
			exp := ev.expected[hash]

			var expectedFloats []FPoint
			var expectedHistograms []HPoint

			for i, e := range exp.vals {
				ts := ev.start.Add(time.Duration(i) * ev.step)

				if ts.After(ev.end) {
					return fmt.Errorf("expected %v points for %s, but query time range cannot return this many points", len(exp.vals), ev.metrics[hash])
				}

				t := ts.UnixNano() / int64(time.Millisecond/time.Nanosecond)

				if e.Histogram != nil {
					expectedHistograms = append(expectedHistograms, HPoint{T: t, H: e.Histogram})
				} else if !e.Omitted {
					expectedFloats = append(expectedFloats, FPoint{T: t, F: e.Value})
				}
			}

			if len(expectedFloats) != len(s.Floats) || len(expectedHistograms) != len(s.Histograms) {
				return fmt.Errorf("expected %v float points and %v histogram points for %s, but got %s", len(expectedFloats), len(expectedHistograms), ev.metrics[hash], formatSeriesResult(s))
			}

			for i, expected := range expectedFloats {
				actual := s.Floats[i]

				if expected.T != actual.T {
					return fmt.Errorf("expected float value at index %v for %s to have timestamp %v, but it had timestamp %v (result has %s)", i, ev.metrics[hash], expected.T, actual.T, formatSeriesResult(s))
				}

				if !almostEqual(actual.F, expected.F, defaultEpsilon) {
					return fmt.Errorf("expected float value at index %v (t=%v) for %s to be %v, but got %v (result has %s)", i, actual.T, ev.metrics[hash], expected.F, actual.F, formatSeriesResult(s))
				}
			}

			for i, expected := range expectedHistograms {
				actual := s.Histograms[i]

				if expected.T != actual.T {
					return fmt.Errorf("expected histogram value at index %v for %s to have timestamp %v, but it had timestamp %v (result has %s)", i, ev.metrics[hash], expected.T, actual.T, formatSeriesResult(s))
				}

				if !actual.H.Equals(expected.H) {
					return fmt.Errorf("expected histogram value at index %v (t=%v) for %s to be %v, but got %v (result has %s)", i, actual.T, ev.metrics[hash], expected.H, actual.H, formatSeriesResult(s))
				}
			}

		}

		for hash := range ev.expected {
			if !seen[hash] {
				return fmt.Errorf("expected metric %s not found", ev.metrics[hash])
			}
		}

	case Vector:
		seen := map[uint64]bool{}
		for pos, v := range val {
			fp := v.Metric.Hash()
			if _, ok := ev.metrics[fp]; !ok {
				return fmt.Errorf("unexpected metric %s in result", v.Metric)
			}
			exp := ev.expected[fp]
			if ev.ordered && exp.pos != pos+1 {
				return fmt.Errorf("expected metric %s with %v at position %d but was at %d", v.Metric, exp.vals, exp.pos, pos+1)
			}
			exp0 := exp.vals[0]
			expH := exp0.Histogram
			if expH == nil && v.H != nil {
				return fmt.Errorf("expected float value %v for %s but got histogram %s", exp0, v.Metric, HistogramTestExpression(v.H))
			}
			if expH != nil && v.H == nil {
				return fmt.Errorf("expected histogram %s for %s but got float value %v", HistogramTestExpression(expH), v.Metric, v.F)
			}
			if expH != nil && !expH.Equals(v.H) {
				return fmt.Errorf("expected %v for %s but got %s", HistogramTestExpression(expH), v.Metric, HistogramTestExpression(v.H))
			}
			if !almostEqual(exp0.Value, v.F, defaultEpsilon) {
				return fmt.Errorf("expected %v for %s but got %v", exp0.Value, v.Metric, v.F)
			}

			seen[fp] = true
		}
		for fp, expVals := range ev.expected {
			if !seen[fp] {
				fmt.Println("vector result", len(val), ev.expr)
				for _, ss := range val {
					fmt.Println("    ", ss.Metric, ss.T, ss.F)
				}
				return fmt.Errorf("expected metric %s with %v not found", ev.metrics[fp], expVals)
			}
		}

	case Scalar:
		if len(ev.expected) != 1 {
			return fmt.Errorf("expected vector result, but got scalar %s", val.String())
		}
		exp0 := ev.expected[0].vals[0]
		if exp0.Histogram != nil {
			return fmt.Errorf("expected Histogram %v but got scalar %s", exp0.Histogram.TestExpression(), val.String())
		}
		if !almostEqual(exp0.Value, val.V, defaultEpsilon) {
			return fmt.Errorf("expected Scalar %v but got %v", val.V, exp0.Value)
		}

	default:
		panic(fmt.Errorf("promql.Test.compareResult: unexpected result type %T", result))
	}
	return nil
}

func formatSeriesResult(s Series) string {
	floatPlural := "s"
	histogramPlural := "s"

	if len(s.Floats) == 1 {
		floatPlural = ""
	}

	if len(s.Histograms) == 1 {
		histogramPlural = ""
	}

	return fmt.Sprintf("%v float point%s %v and %v histogram point%s %v", len(s.Floats), floatPlural, s.Floats, len(s.Histograms), histogramPlural, s.Histograms)
}

// HistogramTestExpression returns TestExpression() for the given histogram or "" if the histogram is nil.
func HistogramTestExpression(h *histogram.FloatHistogram) string {
	if h != nil {
		return h.TestExpression()
	}
	return ""
}

// clearCmd is a command that wipes the test's storage state.
type clearCmd struct{}

func (cmd clearCmd) String() string {
	return "clear"
}

type atModifierTestCase struct {
	expr     string
	evalTime time.Time
}

func atModifierTestCases(exprStr string, evalTime time.Time) ([]atModifierTestCase, error) {
	expr, err := parser.ParseExpr(exprStr)
	if err != nil {
		return nil, err
	}
	ts := timestamp.FromTime(evalTime)

	containsNonStepInvariant := false
	// Setting the @ timestamp for all selectors to be evalTime.
	// If there is a subquery, then the selectors inside it don't get the @ timestamp.
	// If any selector already has the @ timestamp set, then it is untouched.
	parser.Inspect(expr, func(node parser.Node, path []parser.Node) error {
		_, _, subqTs := subqueryTimes(path)
		if subqTs != nil {
			// There is a subquery with timestamp in the path,
			// hence don't change any timestamps further.
			return nil
		}
		switch n := node.(type) {
		case *parser.VectorSelector:
			if n.Timestamp == nil {
				n.Timestamp = makeInt64Pointer(ts)
			}

		case *parser.MatrixSelector:
			if vs := n.VectorSelector.(*parser.VectorSelector); vs.Timestamp == nil {
				vs.Timestamp = makeInt64Pointer(ts)
			}

		case *parser.SubqueryExpr:
			if n.Timestamp == nil {
				n.Timestamp = makeInt64Pointer(ts)
			}

		case *parser.Call:
			_, ok := AtModifierUnsafeFunctions[n.Func.Name]
			containsNonStepInvariant = containsNonStepInvariant || ok
		}
		return nil
	})

	if containsNonStepInvariant {
		// Expression contains a function whose result can vary with evaluation
		// time, even though its arguments are step invariant: skip it.
		return nil, nil
	}

	newExpr := expr.String() // With all the @ evalTime set.
	additionalEvalTimes := []int64{-10 * ts, 0, ts / 5, ts, 10 * ts}
	if ts == 0 {
		additionalEvalTimes = []int64{-1000, -ts, 1000}
	}
	testCases := make([]atModifierTestCase, 0, len(additionalEvalTimes))
	for _, et := range additionalEvalTimes {
		testCases = append(testCases, atModifierTestCase{
			expr:     newExpr,
			evalTime: timestamp.Time(et),
		})
	}

	return testCases, nil
}

// exec processes a single step of the test.
func (t *test) exec(tc testCommand, engine QueryEngine) error {
	switch cmd := tc.(type) {
	case *clearCmd:
		t.clear()

	case *loadCmd:
		app := t.storage.Appender(t.context)
		if err := cmd.append(app); err != nil {
			app.Rollback()
			return err
		}

		if err := app.Commit(); err != nil {
			return err
		}

	case *evalCmd:
		return t.execEval(cmd, engine)

	default:
		panic("promql.Test.exec: unknown test command type")
	}
	return nil
}

func (t *test) execEval(cmd *evalCmd, engine QueryEngine) error {
	if cmd.isRange {
		return t.execRangeEval(cmd, engine)
	}

	return t.execInstantEval(cmd, engine)
}

func (t *test) execRangeEval(cmd *evalCmd, engine QueryEngine) error {
	q, err := engine.NewRangeQuery(t.context, t.storage, nil, cmd.expr, cmd.start, cmd.end, cmd.step)
	if err != nil {
		return err
	}
	res := q.Exec(t.context)
	if res.Err != nil {
		if cmd.fail {
			return nil
		}

		return fmt.Errorf("error evaluating query %q (line %d): %w", cmd.expr, cmd.line, res.Err)
	}
	if res.Err == nil && cmd.fail {
		return fmt.Errorf("expected error evaluating query %q (line %d) but got none", cmd.expr, cmd.line)
	}
	defer q.Close()

	if err := cmd.compareResult(res.Value); err != nil {
		return fmt.Errorf("error in %s %s (line %d): %w", cmd, cmd.expr, cmd.line, err)
	}

	return nil
}

func (t *test) execInstantEval(cmd *evalCmd, engine QueryEngine) error {
	queries, err := atModifierTestCases(cmd.expr, cmd.start)
	if err != nil {
		return err
	}
	queries = append([]atModifierTestCase{{expr: cmd.expr, evalTime: cmd.start}}, queries...)
	for _, iq := range queries {
		q, err := engine.NewInstantQuery(t.context, t.storage, nil, iq.expr, iq.evalTime)
		if err != nil {
			return err
		}
		defer q.Close()
		res := q.Exec(t.context)
		if res.Err != nil {
			if cmd.fail {
				continue
			}
			return fmt.Errorf("error evaluating query %q (line %d): %w", iq.expr, cmd.line, res.Err)
		}
		if res.Err == nil && cmd.fail {
			return fmt.Errorf("expected error evaluating query %q (line %d) but got none", iq.expr, cmd.line)
		}
		err = cmd.compareResult(res.Value)
		if err != nil {
			return fmt.Errorf("error in %s %s (line %d): %w", cmd, iq.expr, cmd.line, err)
		}

		// Check query returns same result in range mode,
		// by checking against the middle step.
		q, err = engine.NewRangeQuery(t.context, t.storage, nil, iq.expr, iq.evalTime.Add(-time.Minute), iq.evalTime.Add(time.Minute), time.Minute)
		if err != nil {
			return err
		}
		rangeRes := q.Exec(t.context)
		if rangeRes.Err != nil {
			return fmt.Errorf("error evaluating query %q (line %d) in range mode: %w", iq.expr, cmd.line, rangeRes.Err)
		}
		defer q.Close()
		if cmd.ordered {
			// Range queries are always sorted by labels, so skip this test case that expects results in a particular order.
			continue
		}
		mat := rangeRes.Value.(Matrix)
		if err := assertMatrixSorted(mat); err != nil {
			return err
		}

		vec := make(Vector, 0, len(mat))
		for _, series := range mat {
			// We expect either Floats or Histograms.
			for _, point := range series.Floats {
				if point.T == timeMilliseconds(iq.evalTime) {
					vec = append(vec, Sample{Metric: series.Metric, T: point.T, F: point.F})
					break
				}
			}
			for _, point := range series.Histograms {
				if point.T == timeMilliseconds(iq.evalTime) {
					vec = append(vec, Sample{Metric: series.Metric, T: point.T, H: point.H})
					break
				}
			}
		}
		if _, ok := res.Value.(Scalar); ok {
			err = cmd.compareResult(Scalar{V: vec[0].F})
		} else {
			err = cmd.compareResult(vec)
		}
		if err != nil {
			return fmt.Errorf("error in %s %s (line %d) range mode: %w", cmd, iq.expr, cmd.line, err)
		}
	}

	return nil
}

func assertMatrixSorted(m Matrix) error {
	if len(m) <= 1 {
		return nil
	}

	for i, s := range m[:len(m)-1] {
		nextIndex := i + 1
		nextMetric := m[nextIndex].Metric

		if labels.Compare(s.Metric, nextMetric) > 0 {
			return fmt.Errorf("matrix results should always be sorted by labels, but matrix is not sorted: series at index %v with labels %s sorts before series at index %v with labels %s", nextIndex, nextMetric, i, s.Metric)
		}
	}

	return nil
}

// clear the current test storage of all inserted samples.
func (t *test) clear() {
	if t.storage != nil {
		err := t.storage.Close()
		require.NoError(t.T, err, "Unexpected error while closing test storage.")
	}
	if t.cancelCtx != nil {
		t.cancelCtx()
	}
	t.storage = teststorage.New(t)
	t.context, t.cancelCtx = context.WithCancel(context.Background())
}

// almostEqual returns true if a and b differ by less than their sum
// multiplied by epsilon.
func almostEqual(a, b, epsilon float64) bool {
	// NaN has no equality but for testing we still want to know whether both values
	// are NaN.
	if math.IsNaN(a) && math.IsNaN(b) {
		return true
	}

	// Cf. http://floating-point-gui.de/errors/comparison/
	if a == b {
		return true
	}

	absSum := math.Abs(a) + math.Abs(b)
	diff := math.Abs(a - b)

	if a == 0 || b == 0 || absSum < minNormal {
		return diff < epsilon*minNormal
	}
	return diff/math.Min(absSum, math.MaxFloat64) < epsilon
}

func parseNumber(s string) (float64, error) {
	n, err := strconv.ParseInt(s, 0, 64)
	f := float64(n)
	if err != nil {
		f, err = strconv.ParseFloat(s, 64)
	}
	if err != nil {
		return 0, fmt.Errorf("error parsing number: %w", err)
	}
	return f, nil
}

// LazyLoader lazily loads samples into storage.
// This is specifically implemented for unit testing of rules.
type LazyLoader struct {
	loadCmd *loadCmd

	storage          storage.Storage
	SubqueryInterval time.Duration

	queryEngine *Engine
	context     context.Context
	cancelCtx   context.CancelFunc

	opts LazyLoaderOpts
}

// LazyLoaderOpts are options for the lazy loader.
type LazyLoaderOpts struct {
	// Both of these must be set to true for regular PromQL (as of
	// Prometheus v2.33). They can still be disabled here for legacy and
	// other uses.
	EnableAtModifier, EnableNegativeOffset bool
}

// NewLazyLoader returns an initialized empty LazyLoader.
func NewLazyLoader(input string, opts LazyLoaderOpts) (*LazyLoader, error) {
	ll := &LazyLoader{
		opts: opts,
	}
	err := ll.parse(input)
	if err != nil {
		return nil, err
	}
	err = ll.clear()
	return ll, err
}

// parse the given load command.
func (ll *LazyLoader) parse(input string) error {
	lines := getLines(input)
	// Accepts only 'load' command.
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if len(l) == 0 {
			continue
		}
		if strings.ToLower(patSpace.Split(l, 2)[0]) == "load" {
			_, cmd, err := parseLoad(lines, i)
			if err != nil {
				return err
			}
			ll.loadCmd = cmd
			return nil
		}

		return raise(i, "invalid command %q", l)
	}
	return errors.New("no \"load\" command found")
}

// clear the current test storage of all inserted samples.
func (ll *LazyLoader) clear() error {
	if ll.storage != nil {
		if err := ll.storage.Close(); err != nil {
			return fmt.Errorf("closing test storage: %w", err)
		}
	}
	if ll.cancelCtx != nil {
		ll.cancelCtx()
	}
	var err error
	ll.storage, err = teststorage.NewWithError()
	if err != nil {
		return err
	}

	opts := EngineOpts{
		Logger:                   nil,
		Reg:                      nil,
		MaxSamples:               10000,
		Timeout:                  100 * time.Second,
		NoStepSubqueryIntervalFn: func(int64) int64 { return durationMilliseconds(ll.SubqueryInterval) },
		EnableAtModifier:         ll.opts.EnableAtModifier,
		EnableNegativeOffset:     ll.opts.EnableNegativeOffset,
	}

	ll.queryEngine = NewEngine(opts)
	ll.context, ll.cancelCtx = context.WithCancel(context.Background())
	return nil
}

// appendTill appends the defined time series to the storage till the given timestamp (in milliseconds).
func (ll *LazyLoader) appendTill(ts int64) error {
	app := ll.storage.Appender(ll.Context())
	for h, smpls := range ll.loadCmd.defs {
		m := ll.loadCmd.metrics[h]
		for i, s := range smpls {
			if s.T > ts {
				// Removing the already added samples.
				ll.loadCmd.defs[h] = smpls[i:]
				break
			}
			if err := appendSample(app, s, m); err != nil {
				return err
			}
			if i == len(smpls)-1 {
				ll.loadCmd.defs[h] = nil
			}
		}
	}
	return app.Commit()
}

// WithSamplesTill loads the samples till given timestamp and executes the given function.
func (ll *LazyLoader) WithSamplesTill(ts time.Time, fn func(error)) {
	tsMilli := ts.Sub(time.Unix(0, 0).UTC()) / time.Millisecond
	fn(ll.appendTill(int64(tsMilli)))
}

// QueryEngine returns the LazyLoader's query engine.
func (ll *LazyLoader) QueryEngine() *Engine {
	return ll.queryEngine
}

// Queryable allows querying the LazyLoader's data.
// Note: only the samples till the max timestamp used
// in `WithSamplesTill` can be queried.
func (ll *LazyLoader) Queryable() storage.Queryable {
	return ll.storage
}

// Context returns the LazyLoader's context.
func (ll *LazyLoader) Context() context.Context {
	return ll.context
}

// Storage returns the LazyLoader's storage.
func (ll *LazyLoader) Storage() storage.Storage {
	return ll.storage
}

// Close closes resources associated with the LazyLoader.
func (ll *LazyLoader) Close() error {
	ll.cancelCtx()
	return ll.storage.Close()
}
