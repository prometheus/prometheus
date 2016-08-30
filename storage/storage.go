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

package storage

import (
	"github.com/prometheus/common/model"
)

type Throttler interface {
	// NeedsThrottling returns true if the underlying storage wishes to not
	// receive any more samples. Append will still work but might lead to
	// undue resource usage. It is recommended to call NeedsThrottling once
	// before an upcoming batch of Append calls (e.g. a full scrape of a
	// target or the evaluation of a rule group) and only proceed with the
	// batch if NeedsThrottling returns false. In that way, the result of a
	// scrape or of an evaluation of a rule group will always be appended
	// completely or not at all, and the work of scraping or evaluation will
	// not be performed in vain. Also, a call of NeedsThrottling is
	// potentially expensive, so limiting the number of calls is reasonable.
	//
	// Only SampleAppenders for which it is considered critical to receive
	// each and every sample should ever return true. SampleAppenders that
	// tolerate not receiving all samples should always return false and
	// instead drop samples as they see fit to avoid overload.
	NeedsThrottling() bool
}

// SampleAppender is the interface to append samples to both, local and remote
// storage. All methods are goroutine-safe.
type SampleAppender interface {
	Throttler
	// Append appends a sample to the underlying storage. Depending on the
	// storage implementation, there are different guarantees for the fate
	// of the sample after Append has returned. Remote storage
	// implementation will simply drop samples if they cannot keep up with
	// sending samples. Local storage implementations will only drop metrics
	// upon unrecoverable errors.
	Append(*model.Sample) error
}

// BatchingSampleAppender is the interface to append samples in a batch
// context, such that it is possible to get a consistent view whereby all
// samples appended in the same batch will appear at the same time.  This does
// not mean that they will be appended atomically, but rather that it is
// possible when querying the local storage implementation to filter out
// samples appended prior to EndBatch().
type BatchingSampleAppender interface {
	SampleAppender
	// EndBatch completes the batch, updating the per-metric scrape watermark.
	// Any subsequent attempt to append new samples will panic.
	EndBatch()
}

// SampleAppenderBatcher is the interface used to create BatchingSampleAppender
// instances.  Although it is technically thread-safe, no two targets should
// ever write to the same batch.
type SampleAppenderBatcher interface {
	Throttler
	StartBatch() BatchingSampleAppender
}

// Fanout is a SampleAppender that appends every sample to each SampleAppender
// in its list.
type Fanout []BatchingSampleAppender

// BatcherFanout is a SampleAppenderBatcher for which each batch created with
// StartBatch will append every sample to each underlying storage implementation.
type BatcherFanout []SampleAppenderBatcher

// StartBatch creates and returns a new batch for each BatchingSampleAppender
// in the list.
func (bf BatcherFanout) StartBatch() BatchingSampleAppender {
	f := make(Fanout, 0, len(bf))
	for _, a := range bf {
		f = append(f, a.StartBatch())
	}
	return f
}

// NeedsThrottling returns true if at least one of the SampleAppenderBatchers in
// the Fanout slice is throttled.
func (bf BatcherFanout) NeedsThrottling() bool {
	for _, a := range bf {
		if a.NeedsThrottling() {
			return true
		}
	}
	return false
}

// Append implements SampleAppender. It appends the provided sample to all
// SampleAppenders in the Fanout slice and waits for each append to complete
// before proceeding with the next.
// If any of the SampleAppenders returns an error, the first one is returned
// at the end.
func (f Fanout) Append(s *model.Sample) error {
	var err error
	for _, a := range f {
		if e := a.Append(s); e != nil && err == nil {
			err = e
		}
	}
	return err
}

// NeedsThrottling returns true if at least one of the SampleAppenders in the
// Fanout slice is throttled.
func (f Fanout) NeedsThrottling() bool {
	for _, a := range f {
		if a.NeedsThrottling() {
			return true
		}
	}
	return false
}

// EndBatch ends the batch for each member of the fanout.
func (f Fanout) EndBatch() {
	for _, a := range f {
		a.EndBatch()
	}
}
