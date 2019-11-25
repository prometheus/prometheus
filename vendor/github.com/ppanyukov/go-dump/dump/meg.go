// Copyright 2019-current Go-dump Authors
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

package dump

import (
	"fmt"
	"math"
)

// meg prints out a number in human readable form, e.g. 20B, 20KB, 20MB, 20GB.
// Returns "NaN" if the input is not a number (int, float).
// Note these values are MB, not MiB.
func meg(n interface{}) string {
	const (
		kilo float64 = 1000.0
		mega float64 = 1000.0 * kilo
		giga float64 = 1000.0 * mega
		tera float64 = 1000.0 * giga
	)

	megInt64 := func(x float64) string {
		xAbs := math.Abs(x)
		switch {
		case xAbs > tera:
			return fmt.Sprintf("%.2fT", x/tera)
		case xAbs > giga:
			return fmt.Sprintf("%.2fG", x/giga)
		case xAbs > mega:
			return fmt.Sprintf("%.2fM", x/mega)
		case xAbs > kilo:
			return fmt.Sprintf("%.2fK", x/kilo)
		default:
			return fmt.Sprintf("%d", int(x))
		}
	}

	// in the order of likelihood
	switch t := n.(type) {
	case uint64:
		return megInt64(float64(t))
	case int64:
		return megInt64(float64(t))

	case int:
		return megInt64(float64(t))

	case float64:
		return megInt64(float64(t))
	case float32:
		return megInt64(float64(t))

	case int32:
		return megInt64(float64(t))
	case uint32:
		return megInt64(float64(t))

	case int16:
		return megInt64(float64(t))

	case uint16:
		return megInt64(float64(t))

	case int8:
		return megInt64(float64(t))
	case uint8: // also byte
		return megInt64(float64(t))

	default:
		return "NaN"
	}
}
