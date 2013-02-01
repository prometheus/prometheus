// Copyright 2013 Prometheus Team
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

package utility

import (
	"time"
)

// A basic interface only useful in testing contexts for dispensing the time
// in a controlled manner.
type InstantProvider interface {
	// The current instant.
	Now() time.Time
}

// Time is a simple means for fluently wrapping around standard Go timekeeping
// mechanisms to enhance testability without compromising code readability.
//
// It is sufficient for use on bare initialization.  A provider should be
// set only for test contexts.  When not provided, it emits the current
// system time.
type Time struct {
	// The underlying means through which time is provided, if supplied.
	Provider InstantProvider
}

// Emit the current instant.
func (t Time) Now() time.Time {
	if t.Provider == nil {
		return time.Now()
	}

	return t.Provider.Now()
}
