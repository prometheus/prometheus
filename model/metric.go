// Copyright 2012 Prometheus Team
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

package model

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"time"
)

type Fingerprint string

type LabelPairs map[string]string
type Metric map[string]string

type SampleValue float32

type Sample struct {
	Labels    LabelPairs
	Value     SampleValue
	Timestamp time.Time
}

type Samples struct {
	Value     SampleValue
	Timestamp time.Time
}

type Interval struct {
	OldestInclusive time.Time
	NewestInclusive time.Time
}

func FingerprintFromString(value string) Fingerprint {
	hash := md5.New()
	io.WriteString(hash, value)
	return Fingerprint(hex.EncodeToString(hash.Sum([]byte{})))
}

func FingerprintFromByteArray(value []byte) Fingerprint {
	hash := md5.New()
	hash.Write(value)
	return Fingerprint(hex.EncodeToString(hash.Sum([]byte{})))
}
