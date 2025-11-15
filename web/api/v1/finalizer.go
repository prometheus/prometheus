// Copyright The Prometheus Authors
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

package v1

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
)

type finalizerKey struct{}

// FinalizerMiddleware creates middleware that allows handlers to set cleanup
// functions that run after response serialization completes.
func FinalizerMiddleware() func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		// Create a pointer to a finalizer function.
		var finalizer *func()
		finalizer = new(func())

		// Store the pointer in context.
		newCtx := context.WithValue(ctx.Context(), finalizerKey{}, finalizer)
		ctx = huma.WithContext(ctx, newCtx)

		// Defer calling the finalizer if it was set.
		// This runs AFTER response serialization.
		defer func() {
			if finalizer != nil && *finalizer != nil {
				(*finalizer)()
			}
		}()

		// Call the next middleware/handler.
		next(ctx)
	}
}

// SetFinalizer sets a cleanup function that will run after response serialization.
// This is useful for closing resources that must remain open until the response is sent.
//
// Example usage:
//
//	qry, err := engine.NewRangeQuery(...)
//	if err != nil {
//	    return nil, err
//	}
//	SetFinalizer(ctx, qry.Close)
//
//	res := qry.Exec(ctx)
//	// ... return response ...
//	// qry.Close() will be called after serialization
func SetFinalizer(ctx context.Context, f func()) {
	if finalizerPtr, ok := ctx.Value(finalizerKey{}).(*func()); ok && finalizerPtr != nil {
		*finalizerPtr = f
	}
}
