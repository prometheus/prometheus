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

package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type flusherKey struct{}

// unwrapper is an interface for response writers that wrap other writers.
type unwrapper interface {
	Unwrap() http.ResponseWriter
}

// SSEMiddleware extracts the http.Flusher and adds it to the context.
// This allows SSE handlers to access the flusher for initial flush.
// It also sets the required headers for SSE connections.
func SSEMiddleware() func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		// Set SSE-specific headers.
		ctx.SetHeader("Cache-Control", "no-cache")
		ctx.SetHeader("Connection", "keep-alive")

		// Extract the flusher and store it in the context.
		w := ctx.BodyWriter()
		for w != nil {
			if flusher, ok := w.(http.Flusher); ok {
				// Add the flusher to the context.
				ctx = huma.WithValue(ctx, flusherKey{}, flusher)
				break
			}
			if u, ok := w.(unwrapper); ok {
				w = u.Unwrap()
			} else {
				break
			}
		}
		next(ctx)
	}
}

// getFlusher retrieves the http.Flusher from the context.
func getFlusher(ctx context.Context) (http.Flusher, bool) {
	flusher, ok := ctx.Value(flusherKey{}).(http.Flusher)
	return flusher, ok
}
