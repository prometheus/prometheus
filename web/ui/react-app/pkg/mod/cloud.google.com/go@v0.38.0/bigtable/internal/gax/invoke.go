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

// Package gax is a snapshot from github.com/googleapis/gax-go/v2 with minor modifications.
package gax

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// APICall is a user defined call stub.
type APICall func(context.Context) error

// scaleDuration returns the product of a and mult.
func scaleDuration(a time.Duration, mult float64) time.Duration {
	ns := float64(a) * mult
	return time.Duration(ns)
}

// invokeWithRetry calls stub using an exponential backoff retry mechanism
// based on the values provided in callSettings.
func invokeWithRetry(ctx context.Context, stub APICall, callSettings CallSettings) error {
	retrySettings := callSettings.RetrySettings
	backoffSettings := callSettings.RetrySettings.BackoffSettings
	delay := backoffSettings.DelayTimeoutSettings.Initial
	for {
		// If the deadline is exceeded...
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := stub(ctx)
		code := grpc.Code(err)
		if code == codes.OK {
			return nil
		}

		if !retrySettings.RetryCodes[code] {
			return err
		}

		delayCtx, _ := context.WithTimeout(ctx, delay)
		<-delayCtx.Done()

		delay = scaleDuration(delay, backoffSettings.DelayTimeoutSettings.Multiplier)
		if delay > backoffSettings.DelayTimeoutSettings.Max {
			delay = backoffSettings.DelayTimeoutSettings.Max
		}
	}
}

// Invoke calls stub with a child of context modified by the specified options.
func Invoke(ctx context.Context, stub APICall, opts ...CallOption) error {
	settings := &CallSettings{}
	callOptions(opts).Resolve(settings)
	if len(settings.RetrySettings.RetryCodes) > 0 {
		return invokeWithRetry(ctx, stub, *settings)
	}
	return stub(ctx)
}
