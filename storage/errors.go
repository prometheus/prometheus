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

package storage

import (
	"errors"
	"fmt"

	"github.com/prometheus/prometheus/model/labels"
)

type errDuplicateSampleForTimestamp struct {
	timestamp           int64
	existing            float64
	existingIsHistogram bool
	newValue            float64
	// duplicateNativeHistogram is true when a native histogram sample at the same timestamp
	// differs from the previously stored histogram value.
	duplicateNativeHistogram bool
	lset                     labels.Labels
}

func NewDuplicateFloatErr(t int64, existing, newValue float64, lset labels.Labels) error {
	return errDuplicateSampleForTimestamp{
		timestamp: t,
		existing:  existing,
		newValue:  newValue,
		lset:      lset,
	}
}

// NewDuplicateHistogramToFloatErr describes an error where a new float sample is sent for same timestamp as previous histogram.
func NewDuplicateHistogramToFloatErr(t int64, newValue float64, lset labels.Labels) error {
	return errDuplicateSampleForTimestamp{
		timestamp:           t,
		existingIsHistogram: true,
		newValue:            newValue,
		lset:                lset,
	}
}

// NewDuplicateNativeHistogramSampleErr is returned when a native histogram sample at timestamp t
// differs from an existing sample at the same timestamp for the series identified by lset.
func NewDuplicateNativeHistogramSampleErr(t int64, lset labels.Labels) error {
	return errDuplicateSampleForTimestamp{
		timestamp:                t,
		duplicateNativeHistogram: true,
		lset:                     lset,
	}
}

// NewDuplicateSampleWithoutTimestampErr describes a duplicate sample when no timestamp was parsed
// for the repeated datapoint (for example duplicate scrape lines without timestamps).
func NewDuplicateSampleWithoutTimestampErr(lset labels.Labels) error {
	return errDuplicateSampleForTimestamp{lset: lset}
}

func (e errDuplicateSampleForTimestamp) seriesSuffix() string {
	if e.lset.IsEmpty() {
		return ""
	}
	return fmt.Sprintf(" for series %s", e.lset.String())
}

func (e errDuplicateSampleForTimestamp) Error() string {
	suffix := e.seriesSuffix()
	switch {
	case e.duplicateNativeHistogram:
		if e.timestamp == 0 {
			return "duplicate native histogram sample for timestamp" + suffix
		}
		return fmt.Sprintf("duplicate native histogram sample for timestamp %d", e.timestamp) + suffix
	case e.timestamp == 0:
		return "duplicate sample for timestamp" + suffix
	case e.existingIsHistogram:
		return fmt.Sprintf("duplicate sample for timestamp %d; overrides not allowed: existing is a histogram, new value %g", e.timestamp, e.newValue) + suffix
	default:
		return fmt.Sprintf("duplicate sample for timestamp %d; overrides not allowed: existing %g, new value %g", e.timestamp, e.existing, e.newValue) + suffix
	}
}

// WrapRemoteWriteAppendErrIfNeeded annotates err with ls when err does not already carry series labels
// (as duplicate-sample errors from TSDB do). Remote Write v2 uses this to avoid a duplicated "for series" suffix.
func WrapRemoteWriteAppendErrIfNeeded(err error, ls labels.Labels) error {
	var e errDuplicateSampleForTimestamp
	if errors.As(err, &e) && !e.lset.IsEmpty() {
		return err
	}
	return fmt.Errorf("%w for series %v", err, ls.String())
}

// Is implements the anonymous interface checked by errors.Is.
// Every errDuplicateSampleForTimestamp compares equal to the global ErrDuplicateSampleForTimestamp.
func (e errDuplicateSampleForTimestamp) Is(t error) bool {
	v, ok := t.(errDuplicateSampleForTimestamp)
	if !ok {
		return false
	}
	if v.isSentinelDuplicateSample() {
		return true
	}
	return e.equal(v)
}

// isSentinelDuplicateSample reports whether e is the zero-value ErrDuplicateSampleForTimestamp.
// errDuplicateSampleForTimestamp cannot be compared with == when labels.Labels is a slice (slicelabels build tag).
func (e errDuplicateSampleForTimestamp) isSentinelDuplicateSample() bool {
	return e.timestamp == 0 &&
		e.existing == 0 &&
		e.newValue == 0 &&
		!e.existingIsHistogram &&
		!e.duplicateNativeHistogram &&
		e.lset.IsEmpty()
}

func (e errDuplicateSampleForTimestamp) equal(v errDuplicateSampleForTimestamp) bool {
	return e.timestamp == v.timestamp &&
		e.existing == v.existing &&
		e.newValue == v.newValue &&
		e.existingIsHistogram == v.existingIsHistogram &&
		e.duplicateNativeHistogram == v.duplicateNativeHistogram &&
		labels.Equal(e.lset, v.lset)
}
