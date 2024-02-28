// Copyright 2024 The Prometheus Authors
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

package testutil

import (
	"fmt"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// Test helper to ensure that the iterator copies histograms when iterating.
// The helper will call it.Next() and check if the next value is a histogram.
func VerifyChunkIteratorCopiesHistograms(h *histogram.Histogram, it chunkenc.Iterator) error {
	originalCount := h.Count
	if it.Next() != chunkenc.ValHistogram {
		return fmt.Errorf("expected histogram value type")
	}
	// Get a copy.
	_, hCopy := it.AtHistogram(nil)
	// Copy into a new histogram.
	hCopy2 := &histogram.Histogram{}
	_, hCopy2 = it.AtHistogram(hCopy2)

	// Change the original
	h.Count = originalCount + 1

	// Ensure that the copy is not affected.
	if hCopy.Count != originalCount {
		return fmt.Errorf("copied histogram count should not change when original is modified")
	}
	if hCopy2.Count != originalCount {
		return fmt.Errorf("copied to histogram count should not change when original is modified")
	}
	return nil
}

// Test helper to ensure that the iterator copies float histograms when iterating.
// The helper will call it.Next() and check if the next value is a float histogram.
func VerifyChunkIteratorCopiesFloatHistograms(fh *histogram.FloatHistogram, it chunkenc.Iterator) error {
	originalCount := fh.Count
	if it.Next() != chunkenc.ValFloatHistogram {
		return fmt.Errorf("expected float histogram value type")
	}
	// Get a copy.
	_, hCopy := it.AtFloatHistogram(nil)
	// Copy into a new histogram.
	hCopy2 := &histogram.FloatHistogram{}
	_, hCopy2 = it.AtFloatHistogram(hCopy2)

	// Change the original
	fh.Count = originalCount + 1

	// Ensure that the copy is not affected.
	if hCopy.Count != originalCount {
		return fmt.Errorf("copied histogram count should not change when original is modified")
	}
	if hCopy2.Count != originalCount {
		return fmt.Errorf("copied to histogram count should not change when original is modified")
	}
	return nil
}
