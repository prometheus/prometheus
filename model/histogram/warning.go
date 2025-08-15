// Copyright 2021 The Prometheus Authors
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

package histogram

// Warning emitted when doing calculations with histograms.
type Warning uint8

const (
	_ Warning = 0 << iota

	// CounterResetHintCollision is emitted when adding or subtraction histograms, where one has
	// hint CounterReset and the other one NotCounterReset.
	CounterResetHintCollision Warning = 1 << iota
)

// Is reports whether the Warning `w` includes any of the flags set in `other`.
// It returns true if there is at least one overlapping bit between `w` and `other`.
func (w Warning) Is(other Warning) bool {
	return w&other != 0
}
