package promql

import (
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/annotations"
)

// ExportedFunctionCalls is an exported version of FunctionCalls that maps function names
// to their implementations. This allows external packages to access PromQL function
// implementations.
//
// Note: Some functions like "info", "label_replace", and "label_join" are nil because
// they are handled specially in the engine and not called via this map.
//
// This map is a direct reference to the internal FunctionCalls map, so any changes
// to FunctionCalls will be reflected here automatically.
//
// Usage example:
//
//	funcCall := promql.ExportedFunctionCalls["abs"]
//	if funcCall != nil {
//	    result, annos := funcCall(vectorVals, matrixVals, args, enh)
//	}
var ExportedFunctionCalls = FunctionCalls

// The following functions are exported wrappers around internal func* functions.
// They provide direct access to individual PromQL function implementations.

// FuncAbs returns the absolute value of all elements in the input vector.
func FuncAbs(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcAbs(vectorVals, matrixVals, args, enh)
}

// FuncAbsent returns 1 if the vector passed to it has no elements, otherwise returns an empty vector.
func FuncAbsent(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcAbsent(vectorVals, matrixVals, args, enh)
}

// FuncAbsentOverTime returns an empty vector if the range vector passed to it has any elements, otherwise returns 1.
func FuncAbsentOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcAbsentOverTime(vectorVals, matrixVals, args, enh)
}

// FuncAcos returns the arccosine of all elements in the input vector.
func FuncAcos(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcAcos(vectorVals, matrixVals, args, enh)
}

// FuncAcosh returns the inverse hyperbolic cosine of all elements in the input vector.
func FuncAcosh(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcAcosh(vectorVals, matrixVals, args, enh)
}

// FuncAsin returns the arcsine of all elements in the input vector.
func FuncAsin(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcAsin(vectorVals, matrixVals, args, enh)
}

// FuncAsinh returns the inverse hyperbolic sine of all elements in the input vector.
func FuncAsinh(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcAsinh(vectorVals, matrixVals, args, enh)
}

// FuncAtan returns the arctangent of all elements in the input vector.
func FuncAtan(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcAtan(vectorVals, matrixVals, args, enh)
}

// FuncAtanh returns the inverse hyperbolic tangent of all elements in the input vector.
func FuncAtanh(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcAtanh(vectorVals, matrixVals, args, enh)
}

// FuncAvgOverTime calculates the average value of all points in the specified interval.
func FuncAvgOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcAvgOverTime(vectorVals, matrixVals, args, enh)
}

// FuncCeil returns the smallest integer greater than or equal to all elements in the input vector.
func FuncCeil(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcCeil(vectorVals, matrixVals, args, enh)
}

// FuncChanges calculates how many times the value has changed within the provided time range.
func FuncChanges(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcChanges(vectorVals, matrixVals, args, enh)
}

// FuncClamp clamps the sample values of all elements in the input vector to be within the range [vMin, vMax].
func FuncClamp(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcClamp(vectorVals, matrixVals, args, enh)
}

// FuncClampMax clamps the sample values of all elements in the input vector to have an upper limit of vMax.
func FuncClampMax(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcClampMax(vectorVals, matrixVals, args, enh)
}

// FuncClampMin clamps the sample values of all elements in the input vector to have a lower limit of vMin.
func FuncClampMin(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcClampMin(vectorVals, matrixVals, args, enh)
}

// FuncCos returns the cosine of all elements in the input vector.
func FuncCos(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcCos(vectorVals, matrixVals, args, enh)
}

// FuncCosh returns the hyperbolic cosine of all elements in the input vector.
func FuncCosh(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcCosh(vectorVals, matrixVals, args, enh)
}

// FuncCountOverTime counts the number of values in the specified interval.
func FuncCountOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcCountOverTime(vectorVals, matrixVals, args, enh)
}

// FuncDaysInMonth returns the number of days in the month for each of the given times in UTC.
func FuncDaysInMonth(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcDaysInMonth(vectorVals, matrixVals, args, enh)
}

// FuncDayOfMonth returns the day of the month for each of the given times in UTC.
func FuncDayOfMonth(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcDayOfMonth(vectorVals, matrixVals, args, enh)
}

// FuncDayOfWeek returns the day of the week for each of the given times in UTC.
func FuncDayOfWeek(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcDayOfWeek(vectorVals, matrixVals, args, enh)
}

// FuncDayOfYear returns the day of the year for each of the given times in UTC.
func FuncDayOfYear(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcDayOfYear(vectorVals, matrixVals, args, enh)
}

// FuncDeg converts radians to degrees for all elements in the input vector.
func FuncDeg(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcDeg(vectorVals, matrixVals, args, enh)
}

// FuncDelta calculates the difference between the first and last value of each time series.
func FuncDelta(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcDelta(vectorVals, matrixVals, args, enh)
}

// FuncDeriv calculates the per-second derivative of the time series.
func FuncDeriv(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcDeriv(vectorVals, matrixVals, args, enh)
}

// FuncDoubleExponentialSmoothing applies double exponential smoothing to the time series.
func FuncDoubleExponentialSmoothing(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcDoubleExponentialSmoothing(vectorVals, matrixVals, args, enh)
}

// FuncExp returns e raised to the power of all elements in the input vector.
func FuncExp(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcExp(vectorVals, matrixVals, args, enh)
}

// FuncFirstOverTime returns the first value of all points in the specified interval.
func FuncFirstOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcFirstOverTime(vectorVals, matrixVals, args, enh)
}

// FuncFloor returns the largest integer less than or equal to all elements in the input vector.
func FuncFloor(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcFloor(vectorVals, matrixVals, args, enh)
}

// FuncHistogramAvg calculates the average of the values in the histogram buckets.
func FuncHistogramAvg(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcHistogramAvg(vectorVals, matrixVals, args, enh)
}

// FuncHistogramCount calculates the count of values in the histogram buckets.
func FuncHistogramCount(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcHistogramCount(vectorVals, matrixVals, args, enh)
}

// FuncHistogramFraction calculates the fraction of values that fall within the provided boundaries.
func FuncHistogramFraction(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcHistogramFraction(vectorVals, matrixVals, args, enh)
}

// FuncHistogramQuantile calculates the φ-quantile (0 ≤ φ ≤ 1) from the buckets of a histogram.
func FuncHistogramQuantile(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcHistogramQuantile(vectorVals, matrixVals, args, enh)
}

// FuncHistogramSum calculates the sum of all values in the histogram buckets.
func FuncHistogramSum(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcHistogramSum(vectorVals, matrixVals, args, enh)
}

// FuncHistogramStdDev calculates the standard deviation of the values in the histogram buckets.
func FuncHistogramStdDev(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcHistogramStdDev(vectorVals, matrixVals, args, enh)
}

// FuncHistogramStdVar calculates the standard variance of the values in the histogram buckets.
func FuncHistogramStdVar(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcHistogramStdVar(vectorVals, matrixVals, args, enh)
}

// FuncHour returns the hour of the day for each of the given times in UTC.
func FuncHour(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcHour(vectorVals, matrixVals, args, enh)
}

// FuncIdelta calculates the difference between the last two samples of the time series.
func FuncIdelta(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcIdelta(vectorVals, matrixVals, args, enh)
}

// FuncIncrease calculates the increase in the time series in the provided time range.
func FuncIncrease(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcIncrease(vectorVals, matrixVals, args, enh)
}

// FuncIrate calculates the per-second instant rate of increase of the time series.
func FuncIrate(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcIrate(vectorVals, matrixVals, args, enh)
}

// FuncLastOverTime returns the last value of all points in the specified interval.
func FuncLastOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcLastOverTime(vectorVals, matrixVals, args, enh)
}

// FuncLn returns the natural logarithm of all elements in the input vector.
func FuncLn(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcLn(vectorVals, matrixVals, args, enh)
}

// FuncLog10 returns the base-10 logarithm of all elements in the input vector.
func FuncLog10(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcLog10(vectorVals, matrixVals, args, enh)
}

// FuncLog2 returns the base-2 logarithm of all elements in the input vector.
func FuncLog2(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcLog2(vectorVals, matrixVals, args, enh)
}

// FuncMadOverTime calculates the median absolute deviation over time.
func FuncMadOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcMadOverTime(vectorVals, matrixVals, args, enh)
}

// FuncMaxOverTime returns the maximum value of all points in the specified interval.
func FuncMaxOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcMaxOverTime(vectorVals, matrixVals, args, enh)
}

// FuncMinOverTime returns the minimum value of all points in the specified interval.
func FuncMinOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcMinOverTime(vectorVals, matrixVals, args, enh)
}

// FuncMinute returns the minute within the hour for each of the given times in UTC.
func FuncMinute(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcMinute(vectorVals, matrixVals, args, enh)
}

// FuncMonth returns the month of the year for each of the given times in UTC.
func FuncMonth(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcMonth(vectorVals, matrixVals, args, enh)
}

// FuncPi returns Pi.
func FuncPi(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcPi(vectorVals, matrixVals, args, enh)
}

// FuncPredictLinear predicts the value of the time series t seconds into the future.
func FuncPredictLinear(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcPredictLinear(vectorVals, matrixVals, args, enh)
}

// FuncPresentOverTime returns 1 if the range vector passed to it has any elements, otherwise returns an empty vector.
func FuncPresentOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcPresentOverTime(vectorVals, matrixVals, args, enh)
}

// FuncQuantileOverTime calculates the φ-quantile (0 ≤ φ ≤ 1) over time.
func FuncQuantileOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcQuantileOverTime(vectorVals, matrixVals, args, enh)
}

// FuncRad converts degrees to radians for all elements in the input vector.
func FuncRad(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcRad(vectorVals, matrixVals, args, enh)
}

// FuncRate calculates the per-second average rate of increase of the time series.
func FuncRate(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcRate(vectorVals, matrixVals, args, enh)
}

// FuncResets calculates the number of counter resets within the provided time range.
func FuncResets(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcResets(vectorVals, matrixVals, args, enh)
}

// FuncRound rounds the sample values of all elements in the input vector to the nearest integer.
func FuncRound(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcRound(vectorVals, matrixVals, args, enh)
}

// FuncScalar converts a single-element input vector into a scalar.
func FuncScalar(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcScalar(vectorVals, matrixVals, args, enh)
}

// FuncSgn returns the sign of all elements in the input vector.
func FuncSgn(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcSgn(vectorVals, matrixVals, args, enh)
}

// FuncSin returns the sine of all elements in the input vector.
func FuncSin(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcSin(vectorVals, matrixVals, args, enh)
}

// FuncSinh returns the hyperbolic sine of all elements in the input vector.
func FuncSinh(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcSinh(vectorVals, matrixVals, args, enh)
}

// FuncSort returns vector elements sorted by their sample values, in ascending order.
func FuncSort(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcSort(vectorVals, matrixVals, args, enh)
}

// FuncSortDesc returns vector elements sorted by their sample values, in descending order.
func FuncSortDesc(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcSortDesc(vectorVals, matrixVals, args, enh)
}

// FuncSortByLabel returns vector elements sorted by the specified label, in ascending order.
func FuncSortByLabel(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcSortByLabel(vectorVals, matrixVals, args, enh)
}

// FuncSortByLabelDesc returns vector elements sorted by the specified label, in descending order.
func FuncSortByLabelDesc(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcSortByLabelDesc(vectorVals, matrixVals, args, enh)
}

// FuncSqrt returns the square root of all elements in the input vector.
func FuncSqrt(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcSqrt(vectorVals, matrixVals, args, enh)
}

// FuncStddevOverTime calculates the population standard deviation over time.
func FuncStddevOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcStddevOverTime(vectorVals, matrixVals, args, enh)
}

// FuncStdvarOverTime calculates the population standard variance over time.
func FuncStdvarOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcStdvarOverTime(vectorVals, matrixVals, args, enh)
}

// FuncSumOverTime calculates the sum of all values in the specified interval.
func FuncSumOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcSumOverTime(vectorVals, matrixVals, args, enh)
}

// FuncTan returns the tangent of all elements in the input vector.
func FuncTan(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcTan(vectorVals, matrixVals, args, enh)
}

// FuncTanh returns the hyperbolic tangent of all elements in the input vector.
func FuncTanh(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcTanh(vectorVals, matrixVals, args, enh)
}

// FuncTime returns the number of seconds since January 1, 1970 UTC.
func FuncTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcTime(vectorVals, matrixVals, args, enh)
}

// FuncTimestamp returns the timestamp of the sample of each series selected.
func FuncTimestamp(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcTimestamp(vectorVals, matrixVals, args, enh)
}

// FuncTsOfFirstOverTime returns the timestamp of the first value of all points in the specified interval.
func FuncTsOfFirstOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcTsOfFirstOverTime(vectorVals, matrixVals, args, enh)
}

// FuncTsOfLastOverTime returns the timestamp of the last value of all points in the specified interval.
func FuncTsOfLastOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcTsOfLastOverTime(vectorVals, matrixVals, args, enh)
}

// FuncTsOfMaxOverTime returns the timestamp of the maximum value of all points in the specified interval.
func FuncTsOfMaxOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcTsOfMaxOverTime(vectorVals, matrixVals, args, enh)
}

// FuncTsOfMinOverTime returns the timestamp of the minimum value of all points in the specified interval.
func FuncTsOfMinOverTime(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcTsOfMinOverTime(vectorVals, matrixVals, args, enh)
}

// FuncVector returns the scalar value as a vector with no labels.
func FuncVector(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcVector(vectorVals, matrixVals, args, enh)
}

// FuncYear returns the year for each of the given times in UTC.
func FuncYear(vectorVals []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return funcYear(vectorVals, matrixVals, args, enh)
}
