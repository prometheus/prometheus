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
	"fmt"

	"code.google.com/p/goprotobuf/proto"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/coding/indexable"
	"github.com/prometheus/prometheus/storage/metric"

	dto "github.com/prometheus/prometheus/model/generated"
)

// SampleKey models the business logic around the data-transfer object
// SampleKey.
type SampleKey struct {
	Fingerprint    *clientmodel.Fingerprint
	FirstTimestamp clientmodel.Timestamp
	LastTimestamp  clientmodel.Timestamp
	SampleCount    uint32
}

// Constrain merges the underlying SampleKey to fit within the keyspace of
// the provided first and last keys and returns whether the key was modified.
func (s *SampleKey) Constrain(first, last *SampleKey) bool {
	switch {
	case s.Before(first.Fingerprint, first.FirstTimestamp):
		*s = *first
		return true
	case last.Before(s.Fingerprint, s.FirstTimestamp):
		*s = *last
		return true
	default:
		return false
	}
}

// Equal returns true if this SampleKey and o have equal fingerprints,
// timestamps, and sample counts.
func (s *SampleKey) Equal(o *SampleKey) bool {
	if s == o {
		return true
	}

	if !s.Fingerprint.Equal(o.Fingerprint) {
		return false
	}
	if !s.FirstTimestamp.Equal(o.FirstTimestamp) {
		return false
	}
	if !s.LastTimestamp.Equal(o.LastTimestamp) {
		return false
	}

	return s.SampleCount == o.SampleCount
}

// MayContain indicates whether the given SampleKey could potentially contain a
// value at the provided time.  Even if true is emitted, that does not mean a
// satisfactory value, in fact, exists.
func (s *SampleKey) MayContain(t clientmodel.Timestamp) bool {
	switch {
	case t.Before(s.FirstTimestamp):
		return false
	case t.After(s.LastTimestamp):
		return false
	default:
		return true
	}
}

// Before returns true if the Fingerprint of this SampleKey is less than fp and
// false if it is greater. If both fingerprints are equal, the FirstTimestamp of
// this SampleKey is checked in the same way against t. If the timestamps are
// eqal, the LastTimestamp of this SampleKey is checked against t (and false is
// returned if they are equal again).
func (s *SampleKey) Before(fp *clientmodel.Fingerprint, t clientmodel.Timestamp) bool {
	if s.Fingerprint.Less(fp) {
		return true
	}
	if !s.Fingerprint.Equal(fp) {
		return false
	}

	if s.FirstTimestamp.Before(t) {
		return true
	}

	return s.LastTimestamp.Before(t)
}

// Dump converts this SampleKey into a DTO for use in serialization purposes.
func (s *SampleKey) Dump(d *dto.SampleKey) {
	d.Reset()
	fp := &dto.Fingerprint{}
	dumpFingerprint(fp, s.Fingerprint)

	d.Fingerprint = fp
	d.Timestamp = indexable.EncodeTime(s.FirstTimestamp)
	d.LastTimestamp = proto.Int64(s.LastTimestamp.Unix())
	d.SampleCount = proto.Uint32(s.SampleCount)
}

func (s *SampleKey) String() string {
	return fmt.Sprintf("SampleKey for %s at %s to %s with %d values.", s.Fingerprint, s.FirstTimestamp, s.LastTimestamp, s.SampleCount)
}

// Load deserializes this SampleKey from a DTO.
func (s *SampleKey) Load(d *dto.SampleKey) {
	f := &clientmodel.Fingerprint{}
	loadFingerprint(f, d.GetFingerprint())
	s.Fingerprint = f
	s.FirstTimestamp = indexable.DecodeTime(d.Timestamp)
	s.LastTimestamp = clientmodel.TimestampFromUnix(d.GetLastTimestamp())
	s.SampleCount = d.GetSampleCount()
}

// ToSampleKey returns the SampleKey for these Values.
func sampleKeyForValues(f *clientmodel.Fingerprint, v metric.Values) *SampleKey {
	return &SampleKey{
		Fingerprint:    f,
		FirstTimestamp: v[0].Timestamp,
		LastTimestamp:  v[len(v)-1].Timestamp,
		SampleCount:    uint32(len(v)),
	}
}
