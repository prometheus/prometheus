// Copyright 2018, OpenCensus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package exemplar implements support for exemplars. Exemplars are additional
// data associated with each measurement.
//
// Their purpose it to provide an example of the kind of thing
// (request, RPC, trace span, etc.) that resulted in that measurement.
package exemplar

import (
	"context"
	"time"
)

const (
	KeyTraceID   = "trace_id"
	KeySpanID    = "span_id"
	KeyPrefixTag = "tag:"
)

// Exemplar is an example data point associated with each bucket of a
// distribution type aggregation.
type Exemplar struct {
	Value       float64     // the value that was recorded
	Timestamp   time.Time   // the time the value was recorded
	Attachments Attachments // attachments (if any)
}

// Attachments is a map of extra values associated with a recorded data point.
// The map should only be mutated from AttachmentExtractor functions.
type Attachments map[string]string

// AttachmentExtractor is a function capable of extracting exemplar attachments
// from the context used to record measurements.
// The map passed to the function should be mutated and returned. It will
// initially be nil: the first AttachmentExtractor that would like to add keys to the
// map is responsible for initializing it.
type AttachmentExtractor func(ctx context.Context, a Attachments) Attachments

var extractors []AttachmentExtractor

// RegisterAttachmentExtractor registers the given extractor associated with the exemplar
// type name.
//
// Extractors will be used to attempt to extract exemplars from the context
// associated with each recorded measurement.
//
// Packages that support exemplars should register their extractor functions on
// initialization.
//
// RegisterAttachmentExtractor should not be called after any measurements have
// been recorded.
func RegisterAttachmentExtractor(e AttachmentExtractor) {
	extractors = append(extractors, e)
}

// NewFromContext extracts exemplars from the given context.
// Each registered AttachmentExtractor (see RegisterAttachmentExtractor) is called in an
// unspecified order to add attachments to the exemplar.
func AttachmentsFromContext(ctx context.Context) Attachments {
	var a Attachments
	for _, extractor := range extractors {
		a = extractor(ctx, a)
	}
	return a
}
