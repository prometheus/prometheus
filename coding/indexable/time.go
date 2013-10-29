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

package indexable

import (
	"encoding/binary"

	clientmodel "github.com/prometheus/client_golang/model"
)

// EncodeTimeInto writes the provided time into the specified buffer subject
// to the LevelDB big endian key sort order requirement.
func EncodeTimeInto(dst []byte, t clientmodel.Timestamp) {
	binary.BigEndian.PutUint64(dst, uint64(t.Unix()))
}

// EncodeTime converts the provided time into a byte buffer subject to the
// LevelDB big endian key sort order requirement.
func EncodeTime(t clientmodel.Timestamp) []byte {
	buffer := make([]byte, 8)

	EncodeTimeInto(buffer, t)

	return buffer
}

// DecodeTime deserializes a big endian byte array into a Unix time in UTC,
// omitting granularity precision less than a second.
func DecodeTime(src []byte) clientmodel.Timestamp {
	return clientmodel.TimestampFromUnix(int64(binary.BigEndian.Uint64(src)))
}
