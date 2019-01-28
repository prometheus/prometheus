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

package trace

import (
	"context"
	"encoding/hex"

	"go.opencensus.io/exemplar"
)

func init() {
	exemplar.RegisterAttachmentExtractor(attachSpanContext)
}

func attachSpanContext(ctx context.Context, a exemplar.Attachments) exemplar.Attachments {
	span := FromContext(ctx)
	if span == nil {
		return a
	}
	sc := span.SpanContext()
	if !sc.IsSampled() {
		return a
	}
	if a == nil {
		a = make(exemplar.Attachments)
	}
	a[exemplar.KeyTraceID] = hex.EncodeToString(sc.TraceID[:])
	a[exemplar.KeySpanID] = hex.EncodeToString(sc.SpanID[:])
	return a
}
