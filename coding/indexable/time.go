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
	"time"
)

var (
	EarliestTime = EncodeTime(time.Unix(0, 0))
)

func EncodeTimeInto(dst []byte, t time.Time) {
	binary.BigEndian.PutUint64(dst, uint64(t.Unix()))
}

func EncodeTime(t time.Time) []byte {
	buffer := make([]byte, 8)

	EncodeTimeInto(buffer, t)

	return buffer
}

func DecodeTime(src []byte) time.Time {
	return time.Unix(int64(binary.BigEndian.Uint64(src)), 0)
}
