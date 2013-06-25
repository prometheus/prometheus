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

package metric

import (
	"fmt"
	"time"

	"code.google.com/p/goprotobuf/proto"

	clientmodel "github.com/prometheus/client_golang/model"

	dto "github.com/prometheus/prometheus/model/generated"

	"github.com/prometheus/prometheus/coding/indexable"
)

// SampleKey models the business logic around the data-transfer object
// SampleKey.
type SampleKey struct {
	Fingerprint    *clientmodel.Fingerprint
	FirstTimestamp time.Time
	LastTimestamp  time.Time
	SampleCount    uint32
}

// MayContain indicates whether the given SampleKey could potentially contain a
// value at the provided time.  Even if true is emitted, that does not mean a
// satisfactory value, in fact, exists.
func (s *SampleKey) MayContain(t time.Time) bool {
	switch {
	case t.Before(s.FirstTimestamp):
		return false
	case t.After(s.LastTimestamp):
		return false
	default:
		return true
	}
}

// ToDTO converts this SampleKey into a DTO for use in serialization purposes.
func (s *SampleKey) Dump(d *dto.SampleKey) {
	d.Reset()
	fp := &dto.Fingerprint{}
	dumpFingerprint(fp, s.Fingerprint)

	d.Fingerprint = fp
	d.Timestamp = indexable.EncodeTime(s.FirstTimestamp)
	d.LastTimestamp = proto.Int64(s.LastTimestamp.Unix())
	d.SampleCount = proto.Uint32(s.SampleCount)
}

// ToPartialDTO converts this SampleKey into a DTO that is only suitable for
// database exploration purposes for a given (Fingerprint, First Sample Time)
// tuple.
func (s *SampleKey) FOOdumpPartial(d *dto.SampleKey) {
	d.Reset()

	f := &dto.Fingerprint{}
	dumpFingerprint(f, s.Fingerprint)

	d.Fingerprint = f
	d.Timestamp = indexable.EncodeTime(s.FirstTimestamp)
}

func (s *SampleKey) String() string {
	return fmt.Sprintf("SampleKey for %s at %s to %s with %d values.", s.Fingerprint, s.FirstTimestamp, s.LastTimestamp, s.SampleCount)
}

func (s *SampleKey) Load(d *dto.SampleKey) {
	f := &clientmodel.Fingerprint{}
	loadFingerprint(f, d.GetFingerprint())
	s.Fingerprint = f
	s.FirstTimestamp = indexable.DecodeTime(d.Timestamp)
	s.LastTimestamp = time.Unix(d.GetLastTimestamp(), 0).UTC()
	s.SampleCount = d.GetSampleCount()
}
