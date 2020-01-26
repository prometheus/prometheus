/*
Copyright 2015 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package gax

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRandomizedDelays(t *testing.T) {
	max := 200 * time.Millisecond
	settings := []CallOption{
		WithRetryCodes([]codes.Code{codes.Unavailable, codes.DeadlineExceeded}),
		WithDelayTimeoutSettings(10*time.Millisecond, max, 1.5),
	}

	deadline := time.Now().Add(1 * time.Second)
	ctx, _ := context.WithDeadline(context.Background(), deadline)
	var invokeTime time.Time
	_ = Invoke(ctx, func(childCtx context.Context) error {
		// Keep failing, make sure we never slept more than max (plus a fudge factor)
		if !invokeTime.IsZero() {
			if got, want := time.Since(invokeTime), max; got > (want + 20*time.Millisecond) {
				t.Logf("Slept too long. Got: %v, want: %v", got, max)
			}
		}
		invokeTime = time.Now()
		// Workaround for `go vet`: https://github.com/grpc/grpc-go/issues/90
		errf := status.Errorf
		return errf(codes.Unavailable, "")
	}, settings...)
}
