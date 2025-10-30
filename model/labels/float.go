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

package labels

import (
	"bytes"
	"math"
	"strconv"
	"sync"
)

// floatFormatBufPool is exclusively used in FormatOpenMetricsFloat.
var floatFormatBufPool = sync.Pool{
	New: func() any {
		// To contain at most 17 digits and additional syntax for a float64.
		b := make([]byte, 0, 24)
		return &b
	},
}

// FormatOpenMetricsFloat works like the usual Go string formatting of a float
// but appends ".0" if the resulting number would otherwise contain neither a
// "." nor an "e".
func FormatOpenMetricsFloat(f float64) string {
	// A few common cases hardcoded.
	switch {
	case f == 1:
		return "1.0"
	case f == 0:
		return "0.0"
	case f == -1:
		return "-1.0"
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, +1):
		return "+Inf"
	case math.IsInf(f, -1):
		return "-Inf"
	}
	bp := floatFormatBufPool.Get().(*[]byte)
	defer floatFormatBufPool.Put(bp)

	*bp = strconv.AppendFloat((*bp)[:0], f, 'g', -1, 64)
	if bytes.ContainsAny(*bp, "e.") {
		return string(*bp)
	}
	*bp = append(*bp, '.', '0')
	return string(*bp)
}
