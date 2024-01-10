// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheus // import "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus"

import "strings"

var wordToUCUM = map[string]string{

	// Time
	"days":         "d",
	"hours":        "h",
	"minutes":      "min",
	"seconds":      "s",
	"milliseconds": "ms",
	"microseconds": "us",
	"nanoseconds":  "ns",

	// Bytes
	"bytes":     "By",
	"kibibytes": "KiBy",
	"mebibytes": "MiBy",
	"gibibytes": "GiBy",
	"tibibytes": "TiBy",
	"kilobytes": "KBy",
	"megabytes": "MBy",
	"gigabytes": "GBy",
	"terabytes": "TBy",

	// SI
	"meters":  "m",
	"volts":   "V",
	"amperes": "A",
	"joules":  "J",
	"watts":   "W",
	"grams":   "g",

	// Misc
	"celsius": "Cel",
	"hertz":   "Hz",
	"ratio":   "1",
	"percent": "%",
}

// The map that translates the "per" unit
// Example: per_second (singular) => /s
var perWordToUCUM = map[string]string{
	"second": "s",
	"minute": "m",
	"hour":   "h",
	"day":    "d",
	"week":   "w",
	"month":  "mo",
	"year":   "y",
}

// UnitWordToUCUM converts english unit words to UCUM units:
// https://ucum.org/ucum#section-Alphabetic-Index-By-Symbol
// It also handles rates, such as meters_per_second, by translating the first
// word to UCUM, and the "per" word to UCUM. It joins them with a "/" between.
func UnitWordToUCUM(unit string) string {
	unitTokens := strings.SplitN(unit, "_per_", 2)
	if len(unitTokens) == 0 {
		return ""
	}
	ucumUnit := wordToUCUMOrDefault(unitTokens[0])
	if len(unitTokens) > 1 && unitTokens[1] != "" {
		ucumUnit += "/" + perWordToUCUMOrDefault(unitTokens[1])
	}
	return ucumUnit
}

// wordToUCUMOrDefault retrieves the Prometheus "basic" unit corresponding to
// the specified "basic" unit. Returns the specified unit if not found in
// wordToUCUM.
func wordToUCUMOrDefault(unit string) string {
	if promUnit, ok := wordToUCUM[unit]; ok {
		return promUnit
	}
	return unit
}

// perWordToUCUMOrDefault retrieve the Prometheus "per" unit corresponding to
// the specified "per" unit. Returns the specified unit if not found in perWordToUCUM.
func perWordToUCUMOrDefault(perUnit string) string {
	if promPerUnit, ok := perWordToUCUM[perUnit]; ok {
		return promPerUnit
	}
	return perUnit
}
