package promql

import (
	"context"
	"log/slog"
	"reflect"
	"time"
	"unsafe"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/stats"
)

// AggregationParams contains parameters for aggregation operations.
// This is a wrapper around the internal evaluator state needed for aggregations.
type AggregationParams struct {
	// StartTimestamp is the start time in milliseconds.
	StartTimestamp int64
	// EndTimestamp is the end time in milliseconds.
	EndTimestamp int64
	// Interval is the interval in milliseconds.
	Interval int64
}

// GroupedAggregation represents the state of an aggregation group.
// This is an exported wrapper around the internal groupedAggregation type.
// The memory layout matches groupedAggregation exactly, allowing safe conversion.
type GroupedAggregation struct {
	FloatValue     float64
	HistogramValue *histogram.FloatHistogram
	FloatMean      float64
	FloatKahanC    float64 // "Compensating value" for Kahan summation.
	GroupCount     float64
	Heap           Vector // vectorByValueHeap is an alias of Vector

	// All bools together for better packing within the struct.
	Seen                   bool // Was this output groups seen in the input at this timestamp.
	HasFloat               bool // Has at least 1 float64 sample aggregated.
	HasHistogram           bool // Has at least 1 histogram sample aggregated.
	IncompatibleHistograms bool // If true, group has seen mixed exponential and custom buckets.
	GroupAggrComplete      bool // Used by LIMITK to short-cut series loop when we've reached K elem on every group.
	IncrementalMean        bool // True after reverting to incremental calculation of the mean value.
	CounterResetSeen       bool // Counter reset hint CounterReset seen. Currently only used for histogram samples.
	NotCounterResetSeen    bool // Counter reset hint NotCounterReset seen. Currently only used for histogram samples.
	DropName               bool // True if any sample in this group has DropName set.
}

// convertToInternal converts []GroupedAggregation to []groupedAggregation.
// This is safe because both types have identical memory layouts.
func convertToInternal(groups []GroupedAggregation) []groupedAggregation {
	if len(groups) == 0 {
		return nil
	}
	// Use unsafe conversion since the memory layouts are identical
	return *(*[]groupedAggregation)(unsafe.Pointer(&groups))
}

// convertFromInternal converts []groupedAggregation to []GroupedAggregation.
// This is safe because both types have identical memory layouts.
func convertFromInternal(groups []groupedAggregation) []GroupedAggregation {
	if len(groups) == 0 {
		return nil
	}
	// Use unsafe conversion since the memory layouts are identical
	return *(*[]GroupedAggregation)(unsafe.Pointer(&groups))
}

// Aggregation performs aggregation operations (sum, avg, count, stdvar, stddev, quantile)
// at one timestep on inputMatrix.
//
// Parameters:
//   - e: The aggregate expression containing the operation type
//   - q: The quantile value (for quantile operation) or parameter value
//   - inputMatrix: The input matrix to aggregate
//   - outputMatrix: Pre-populated output matrix with grouping labels (one-to-one with groups)
//   - seriesToResult: Maps inputMatrix indexes to outputMatrix indexes
//   - groups: Aggregation groups (one-to-one with outputMatrix)
//   - enh: Evaluation node helper containing timestamp and output vector
//
// Returns annotations from the aggregation operation.
//
// Note: This is an exported wrapper around the internal evaluator.aggregation method.
// The outputMatrix and groups slices are modified in-place.
// The method uses ev.nextValues internally, which doesn't require evaluator state,
// so a minimal evaluator instance is sufficient.
func Aggregation(
	e *parser.AggregateExpr,
	q float64,
	inputMatrix, outputMatrix Matrix,
	seriesToResult []int,
	groups []GroupedAggregation,
	enh *EvalNodeHelper,
	opts *EvaluatorOpts,
) annotations.Annotations {
	// Convert exported type to internal type
	internalGroups := convertToInternal(groups)

	// Create a minimal evaluator instance.
	// The aggregation method calls ev.nextValues, which doesn't use evaluator state,
	// so we only need a valid pointer.
	var ev *evaluator
	if opts != nil {
		// Option 1: Use NewEvaluator + unsafe conversion (recommended, better performance)
		eval := NewEvaluator(*opts)
		ev = eval.toInternal()

		// Option 2: Use reflection (alternative, but slower)
		// ev = createEvaluatorFromOpts(*opts)
	} else {
		// This method doesn't use evaluator state, so we can use a nil receiver.
		ev = (*evaluator)(nil)
	}

	// Call the internal method
	annos := ev.aggregation(e, q, inputMatrix, outputMatrix, seriesToResult, internalGroups, enh)

	// Note: Since we're using unsafe conversion, modifications to internalGroups
	// are already reflected in the original groups slice (they share the same memory).
	// No explicit conversion back is needed.

	return annos
}

// AggregationK performs K-based aggregation operations (topk, bottomk, limitk, limit_ratio)
// at one timestep on inputMatrix.
//
// Parameters:
//   - e: The aggregate expression containing the operation type
//   - fParam: The parameter value (k for topk/bottomk/limitk, ratio for limit_ratio)
//   - inputMatrix: The input matrix to aggregate
//   - seriesToResult: Maps inputMatrix indexes to groups indexes
//   - groups: Aggregation groups
//   - enh: Evaluation node helper containing timestamp
//   - seriess: Map for storing results (for range queries)
//   - params: Aggregation parameters containing timestamp information
//
// Returns the result matrix and annotations.
//
// Note: For instant queries (startTimestamp == endTimestamp), returns a Matrix.
// For range queries, aggregates output in the seriess map.
func AggregationK(
	e *parser.AggregateExpr,
	fParam float64,
	inputMatrix Matrix,
	seriesToResult []int,
	groups []GroupedAggregation,
	enh *EvalNodeHelper,
	seriess map[uint64]Series,
	params AggregationParams,
	opts *EvaluatorOpts,
) (Matrix, annotations.Annotations) {
	// Convert exported type to internal type
	internalGroups := convertToInternal(groups)

	// Create an evaluator with the required timestamp fields.
	// aggregationK uses ev.endTimestamp and ev.nextValues internally.
	var ev *evaluator
	if opts != nil {
		eval := NewEvaluator(*opts)
		ev = eval.toInternal()
	} else {
		ev = (*evaluator)(nil)
	}

	// Call the internal method
	mat, annos := ev.aggregationK(e, fParam, inputMatrix, seriesToResult, internalGroups, enh, seriess)

	// Note: Since we're using unsafe conversion, modifications to internalGroups
	// are already reflected in the original groups slice (they share the same memory).
	// No explicit conversion back is needed.

	return mat, annos
}

// createEvaluatorFromOpts creates an evaluator instance from EvaluatorOpts using reflection.
// This is an alternative to NewEvaluator + unsafe conversion, but has performance overhead.
// For better performance, use NewEvaluator + toInternal() instead.
func createEvaluatorFromOpts(opts EvaluatorOpts) *evaluator {
	// Create a new evaluator struct
	ev := &evaluator{}

	// Use reflection to set fields from opts
	optsValue := reflect.ValueOf(opts)
	evValue := reflect.ValueOf(ev).Elem()

	// Map field names from EvaluatorOpts to evaluator
	fieldMap := map[string]string{
		"StartTimestamp":           "startTimestamp",
		"EndTimestamp":             "endTimestamp",
		"Interval":                 "interval",
		"MaxSamples":               "maxSamples",
		"Logger":                   "logger",
		"LookbackDelta":            "lookbackDelta",
		"SamplesStats":             "samplesStats",
		"NoStepSubqueryIntervalFn": "noStepSubqueryIntervalFn",
		"EnableDelayedNameRemoval": "enableDelayedNameRemoval",
		"EnableTypeAndUnitLabels":  "enableTypeAndUnitLabels",
		"Querier":                  "querier",
	}

	optsType := optsValue.Type()
	for i := 0; i < optsType.NumField(); i++ {
		optsField := optsType.Field(i)
		if internalFieldName, ok := fieldMap[optsField.Name]; ok {
			optsFieldValue := optsValue.Field(i)
			if !optsFieldValue.IsZero() {
				evField := evValue.FieldByName(internalFieldName)
				if evField.IsValid() && evField.CanSet() {
					evField.Set(optsFieldValue)
				}
			}
		}
	}

	return ev
}

// AggregationCountValues evaluates count_values aggregation on a vector.
//
// Parameters:
//   - e: The aggregate expression
//   - grouping: The grouping labels
//   - valueLabel: The label name for the value
//   - vec: The input vector
//   - enh: Evaluation node helper
//   - opts: Optional evaluator options. If nil, a nil evaluator receiver is used
//     (which is sufficient since aggregationCountValues doesn't use evaluator state).
//
// Returns the result vector and annotations.
//
// Note: This outputs as many series per group as there are values in the input.
// This method doesn't use evaluator state, so a nil receiver is sufficient.
// However, if opts is provided, an Evaluator instance will be created for consistency.
//
// Implementation note: When opts is provided, we use NewEvaluator + unsafe conversion
// for better performance. Alternatively, createEvaluatorFromOpts uses reflection but
// has performance overhead.
func AggregationCountValues(
	e *parser.AggregateExpr,
	grouping []string,
	valueLabel string,
	vec Vector,
	enh *EvalNodeHelper,
	opts *EvaluatorOpts,
) (Vector, annotations.Annotations) {
	var ev *evaluator
	if opts != nil {
		// Option 1: Use NewEvaluator + unsafe conversion (recommended, better performance)
		eval := NewEvaluator(*opts)
		ev = eval.toInternal()

		// Option 2: Use reflection (alternative, but slower)
		// ev = createEvaluatorFromOpts(*opts)
	} else {
		// This method doesn't use evaluator state, so we can use a nil receiver.
		ev = (*evaluator)(nil)
	}
	return ev.aggregationCountValues(e, grouping, valueLabel, vec, enh)
}

// EvaluatorOpts contains options for creating a new Evaluator.
type EvaluatorOpts struct {
	// StartTimestamp is the start time in milliseconds.
	StartTimestamp int64
	// EndTimestamp is the end time in milliseconds.
	EndTimestamp int64
	// Interval is the interval in milliseconds.
	Interval int64

	// MaxSamples is the maximum number of samples allowed.
	MaxSamples int
	// Logger is the logger to use for evaluation.
	Logger *slog.Logger
	// LookbackDelta is the lookback delta duration.
	LookbackDelta time.Duration
	// SamplesStats is the query samples statistics tracker.
	SamplesStats *stats.QuerySamples
	// NoStepSubqueryIntervalFn is the function to compute subquery interval when no step is specified.
	NoStepSubqueryIntervalFn func(rangeMillis int64) int64
	// EnableDelayedNameRemoval enables delayed removal of __name__ label.
	EnableDelayedNameRemoval bool
	// EnableTypeAndUnitLabels enables type and unit label support.
	EnableTypeAndUnitLabels bool
	// Querier is the storage querier to use for data retrieval.
	Querier storage.Querier
}

// Evaluator is an exported wrapper around the internal evaluator type.
// It evaluates PromQL expressions over fixed timestamps.
//
// The memory layout matches the internal evaluator struct exactly, allowing
// safe conversion between the two types.
// IMPORTANT: Field order must match evaluator struct exactly for unsafe conversion to work.
type Evaluator struct {
	// StartTimestamp is the start time in milliseconds.
	StartTimestamp int64
	// EndTimestamp is the end time in milliseconds.
	EndTimestamp int64
	// Interval is the interval in milliseconds.
	Interval int64

	// MaxSamples is the maximum number of samples allowed.
	MaxSamples int
	// CurrentSamples is the current number of samples in memory.
	CurrentSamples int
	// Logger is the logger to use for evaluation.
	Logger *slog.Logger
	// LookbackDelta is the lookback delta duration.
	LookbackDelta time.Duration
	// SamplesStats is the query samples statistics tracker.
	SamplesStats *stats.QuerySamples
	// NoStepSubqueryIntervalFn is the function to compute subquery interval when no step is specified.
	NoStepSubqueryIntervalFn func(rangeMillis int64) int64
	// EnableDelayedNameRemoval enables delayed removal of __name__ label.
	EnableDelayedNameRemoval bool
	// EnableTypeAndUnitLabels enables type and unit label support.
	EnableTypeAndUnitLabels bool
	// Querier is the storage querier to use for data retrieval.
	Querier storage.Querier
}

// NewEvaluator creates a new Evaluator with the given options.
func NewEvaluator(opts EvaluatorOpts) *Evaluator {
	return &Evaluator{
		StartTimestamp:           opts.StartTimestamp,
		EndTimestamp:             opts.EndTimestamp,
		Interval:                 opts.Interval,
		MaxSamples:               opts.MaxSamples,
		Logger:                   opts.Logger,
		LookbackDelta:            opts.LookbackDelta,
		SamplesStats:             opts.SamplesStats,
		NoStepSubqueryIntervalFn: opts.NoStepSubqueryIntervalFn,
		EnableDelayedNameRemoval: opts.EnableDelayedNameRemoval,
		EnableTypeAndUnitLabels:  opts.EnableTypeAndUnitLabels,
		Querier:                  opts.Querier,
	}
}

// toInternal converts the exported Evaluator to the internal evaluator type.
// This is safe because both types have identical memory layouts.
func (e *Evaluator) toInternal() *evaluator {
	if e == nil {
		return nil
	}
	// Use unsafe conversion since the memory layouts are identical
	return (*evaluator)(unsafe.Pointer(e))
}

// fromInternal creates an Evaluator from an internal evaluator.
// This is safe because both types have identical memory layouts.
func evaluatorFromInternal(ev *evaluator) *Evaluator {
	if ev == nil {
		return nil
	}
	// Use unsafe conversion since the memory layouts are identical
	return (*Evaluator)(unsafe.Pointer(ev))
}

// Eval evaluates the given expression and returns the result value, warnings, and error.
//
// This is an exported wrapper around the internal evaluator.Eval method.
func (e *Evaluator) Eval(ctx context.Context, expr parser.Expr) (parser.Value, annotations.Annotations, error) {
	ev := e.toInternal()
	return ev.Eval(ctx, expr)
}
