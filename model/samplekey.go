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

package model

import (
	"code.google.com/p/goprotobuf/proto"
	"github.com/prometheus/prometheus/coding/indexable"
	dto "github.com/prometheus/prometheus/model/generated"
	"time"
)

// SampleKey models the business logic around the data-transfer object
// SampleKey.
type SampleKey struct {
	Fingerprint    Fingerprint
	FirstTimestamp time.Time
	LastTimestamp  time.Time
	SampleCount    uint32
}

// MayContain indicates whether the given SampleKey could potentially contain a
// value at the provided time.  Even if true is emitted, that does not mean a
// satisfactory value, in fact, exists.
func (s SampleKey) MayContain(t time.Time) (could bool) {
	switch {
	case t.Before(s.FirstTimestamp):
		return
	case t.After(s.LastTimestamp):
		return
	}

	return true
}

// ToDTO converts this SampleKey into a DTO for use in serialization purposes.
func (s SampleKey) ToDTO() (out *dto.SampleKey) {
	out = &dto.SampleKey{
		Fingerprint:   s.Fingerprint.ToDTO(),
		Timestamp:     indexable.EncodeTime(s.FirstTimestamp),
		LastTimestamp: proto.Int64(s.LastTimestamp.Unix()),
		SampleCount:   proto.Uint32(s.SampleCount),
	}

	return
}

// ToPartialDTO converts this SampleKey into a DTO that is only suitable for
// database exploration purposes for a given (Fingerprint, First Sample Time)
// tuple.
func (s SampleKey) ToPartialDTO(out *dto.SampleKey) {
	out = &dto.SampleKey{
		Fingerprint: s.Fingerprint.ToDTO(),
		Timestamp:   indexable.EncodeTime(s.FirstTimestamp),
	}

	return
}

// NewSampleKeyFromDTO builds a new SampleKey from a provided data-transfer
// object.
func NewSampleKeyFromDTO(dto *dto.SampleKey) SampleKey {
	return SampleKey{
		Fingerprint:    NewFingerprintFromDTO(dto.Fingerprint),
		FirstTimestamp: indexable.DecodeTime(dto.Timestamp),
		LastTimestamp:  time.Unix(*dto.LastTimestamp, 0),
		SampleCount:    *dto.SampleCount,
	}
}
