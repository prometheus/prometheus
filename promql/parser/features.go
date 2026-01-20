// Copyright The Prometheus Authors
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

package parser

import "github.com/prometheus/prometheus/util/features"

// RegisterFeatures registers all PromQL features with the feature registry.
// This includes operators (arithmetic and comparison/set), aggregators (standard
// and experimental), and functions.
func RegisterFeatures(r features.Collector) {
	// Register core PromQL language keywords.
	for keyword, itemType := range key {
		if itemType.IsKeyword() {
			// Handle experimental keywords separately.
			switch keyword {
			case "anchored", "smoothed":
				r.Set(features.PromQL, keyword, EnableExtendedRangeSelectors)
			case "fill", "fill_left", "fill_right":
				r.Set(features.PromQL, keyword, EnableBinopFillModifiers)
			default:
				r.Enable(features.PromQL, keyword)
			}
		}
	}

	// Register operators.
	for o := ItemType(operatorsStart + 1); o < operatorsEnd; o++ {
		if o.IsOperator() {
			r.Set(features.PromQLOperators, o.String(), true)
		}
	}

	// Register aggregators.
	for a := ItemType(aggregatorsStart + 1); a < aggregatorsEnd; a++ {
		if a.IsAggregator() {
			experimental := a.IsExperimentalAggregator() && !EnableExperimentalFunctions
			r.Set(features.PromQLOperators, a.String(), !experimental)
		}
	}

	// Register functions.
	for f, fc := range Functions {
		r.Set(features.PromQLFunctions, f, !fc.Experimental || EnableExperimentalFunctions)
	}

	// Register experimental parser features.
	r.Set(features.PromQL, "duration_expr", ExperimentalDurationExpr)
}
