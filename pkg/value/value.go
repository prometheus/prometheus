// Copyright 2016 The Prometheus Authors
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

package value

import (
	"math"
)

const (
	NormalNaN uint64 = 0x7ff8000000000001 // A quiet NaN. This is also math.NaN().
	StaleNaN  uint64 = 0x7ff4000000000000 // A signalling NaN, starting 01 to allow for expansion.
)

func IsStaleNaN(v float64) bool {
	return math.Float64bits(v) == StaleNaN
}
