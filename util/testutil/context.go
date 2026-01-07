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

package testutil

import (
	"context"
	"time"

	"go.uber.org/atomic"
)

// A MockContext provides a simple stub implementation of a Context.
type MockContext struct {
	Error  error
	DoneCh chan struct{}
}

// Deadline always will return not set.
func (*MockContext) Deadline() (deadline time.Time, ok bool) {
	return time.Time{}, false
}

// Done returns a read channel for listening to the Done event.
func (c *MockContext) Done() <-chan struct{} {
	return c.DoneCh
}

// Err returns the error, is nil if not set.
func (c *MockContext) Err() error {
	return c.Error
}

// Value ignores the Value and always returns nil.
func (*MockContext) Value(any) any {
	return nil
}

// MockContextErrAfter is a MockContext that will return an error after a certain
// number of calls to Err().
type MockContextErrAfter struct {
	count atomic.Uint64
	MockContext
	FailAfter uint64
}

func (c *MockContextErrAfter) Err() error {
	c.count.Inc()
	if c.count.Load() >= c.FailAfter {
		return context.Canceled
	}
	return c.MockContext.Err()
}

func (c *MockContextErrAfter) Count() uint64 {
	return c.count.Load()
}
