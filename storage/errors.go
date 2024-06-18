// Copyright 2014 The Prometheus Authors
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

import "fmt"

type errDuplicateSampleForTimestamp struct {
	timestamp int64
	existing  float64
	newValue  float64
}

func NewDuplicateFloatErr(t int64, existing, newValue float64) error {
	return errDuplicateSampleForTimestamp{
		timestamp: t,
		existing:  existing,
		newValue:  newValue,
	}
}

func (e errDuplicateSampleForTimestamp) Error() string {
	if e.timestamp == 0 {
		return "duplicate sample for timestamp"
	}
	return fmt.Sprintf("duplicate sample for timestamp %d; overrides not allowed: existing %g, new value %g", e.timestamp, e.existing, e.newValue)
}

// Every errDuplicateSampleForTimestamp compares equal to the global ErrDuplicateSampleForTimestamp.
func (e errDuplicateSampleForTimestamp) Is(t error) bool {
	if t == ErrDuplicateSampleForTimestamp {
		return true
	}
	if v, ok := t.(errDuplicateSampleForTimestamp); ok {
		return e == v
	}
	return false
}
