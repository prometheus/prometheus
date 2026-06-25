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

package prometheusremotewrite

import (
	"errors"
	"fmt"

	"github.com/prometheus/prometheus/util/annotations"
)

// WarningCategory is a stable, machine-readable classification of an OTLP
// translation warning. It is a closed enum so that callers can use it as a
// metric label without risking unbounded cardinality.
type WarningCategory string

const (
	// WarningCategoryLabelNameCollision is set when two or more OTLP attribute
	// names map to the same label name after sanitization and their values are
	// concatenated.
	WarningCategoryLabelNameCollision WarningCategory = "label_name_collision"
	// WarningCategoryHistogramZeroCountNonZeroSum is set when a histogram data
	// point has a zero count but a non-zero sum, for both exponential and
	// classic (NHCB) histograms.
	WarningCategoryHistogramZeroCountNonZeroSum WarningCategory = "histogram_zero_count_non_zero_sum"
	// WarningCategoryOther is the fallback for any warning that has not been
	// assigned a category. It keeps the label space bounded.
	WarningCategoryOther WarningCategory = "other"
)

// categorizedWarning is an annotation error that carries a machine-readable
// category. Its Error() returns the human-readable message unchanged, so
// annotation deduplication (which keys on the message), logging, and
// Annotations.AsStrings all behave exactly as they did for a bare error.
type categorizedWarning struct {
	category WarningCategory
	msg      string
}

func (w *categorizedWarning) Error() string { return w.msg }

// newCategorizedWarningf builds a categorized warning whose message is formatted
// from format and args, identically to fmt.Errorf.
func newCategorizedWarningf(category WarningCategory, format string, args ...any) *categorizedWarning {
	return &categorizedWarning{category: category, msg: fmt.Sprintf(format, args...)}
}

// WarningCategoryOf returns the category of an annotation error, or
// WarningCategoryOther if it carries none.
func WarningCategoryOf(err error) WarningCategory {
	var cw *categorizedWarning
	if errors.As(err, &cw) {
		return cw.category
	}
	return WarningCategoryOther
}

// CountWarningsByCategory buckets the warnings in annots by category, skipping
// info-level annotations.
func CountWarningsByCategory(annots annotations.Annotations) map[WarningCategory]int {
	if len(annots) == 0 {
		return nil
	}
	counts := make(map[WarningCategory]int, len(annots))
	for _, err := range annots {
		if errors.Is(err, annotations.PromQLInfo) {
			continue
		}
		counts[WarningCategoryOf(err)]++
	}
	return counts
}
