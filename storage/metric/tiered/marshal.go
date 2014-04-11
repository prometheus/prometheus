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

package tiered

import (
	"encoding/binary"
	"math"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

const (
	// sampleSize is the number of bytes per sample in marshalled format.
	sampleSize = 16
	// formatVersion is used as a version marker in the marshalled format.
	formatVersion = 1
	// formatVersionSize is the number of bytes used by the serialized formatVersion.
	formatVersionSize = 1
)

// marshal marshals a group of samples for being written to disk into dest or a
// new slice if dest has insufficient capacity.
func marshalValues(v metric.Values, dest []byte) []byte {
	sz := formatVersionSize + len(v)*sampleSize
	if cap(dest) < sz {
		dest = make([]byte, sz)
	} else {
		dest = dest[0:sz]
	}

	dest[0] = formatVersion
	for i, val := range v {
		offset := formatVersionSize + i*sampleSize
		binary.LittleEndian.PutUint64(dest[offset:], uint64(val.Timestamp.Unix()))
		binary.LittleEndian.PutUint64(dest[offset+8:], math.Float64bits(float64(val.Value)))
	}
	return dest
}

// unmarshalValues decodes marshalled samples into dest and returns either dest
// or a new slice containing those values if dest has insufficient capacity.
func unmarshalValues(buf []byte, dest metric.Values) metric.Values {
	if buf[0] != formatVersion {
		panic("unsupported format version")
	}

	n := (len(buf) - formatVersionSize) / sampleSize

	if cap(dest) < n {
		dest = make(metric.Values, n)
	} else {
		dest = dest[0:n]
	}

	for i := 0; i < n; i++ {
		offset := formatVersionSize + i*sampleSize
		dest[i].Timestamp = clientmodel.TimestampFromUnix(int64(binary.LittleEndian.Uint64(buf[offset:])))
		dest[i].Value = clientmodel.SampleValue(math.Float64frombits(binary.LittleEndian.Uint64(buf[offset+8:])))
	}
	return dest
}
