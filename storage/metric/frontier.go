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

	clientmodel "github.com/prometheus/client_golang/model"

	dto "github.com/prometheus/prometheus/model/generated"

	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/coding/indexable"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
)

// diskFrontier describes an on-disk store of series to provide a
// representation of the known keyspace and time series values available.
//
// This is used to reduce the burden associated with LevelDB iterator
// management.
type diskFrontier struct {
	firstFingerprint *clientmodel.Fingerprint
	firstSupertime   time.Time
	lastFingerprint  *clientmodel.Fingerprint
	lastSupertime    time.Time
}

func (f diskFrontier) String() string {
	return fmt.Sprintf("diskFrontier from %s at %s to %s at %s", f.firstFingerprint, f.firstSupertime, f.lastFingerprint, f.lastSupertime)
}

func (f *diskFrontier) ContainsFingerprint(fingerprint *clientmodel.Fingerprint) bool {
	return !(fingerprint.Less(f.firstFingerprint) || f.lastFingerprint.Less(fingerprint))
}

func newDiskFrontier(i leveldb.Iterator) (d *diskFrontier, present bool, err error) {
	if !i.SeekToLast() || i.Key() == nil {
		return nil, false, nil
	}
	lastKey, err := extractSampleKey(i)
	if err != nil {
		return nil, false, err
	}

	if !i.SeekToFirst() || i.Key() == nil {
		return nil, false, nil
	}
	firstKey, err := extractSampleKey(i)
	if err != nil {
		return nil, false, err
	}

	return &diskFrontier{
		firstFingerprint: firstKey.Fingerprint,
		firstSupertime:   firstKey.FirstTimestamp,
		lastFingerprint:  lastKey.Fingerprint,
		lastSupertime:    lastKey.FirstTimestamp,
	}, true, nil
}

// seriesFrontier represents the valid seek frontier for a given series.
type seriesFrontier struct {
	firstSupertime time.Time
	lastSupertime  time.Time
	lastTime       time.Time
}

func (f *seriesFrontier) String() string {
	return fmt.Sprintf("seriesFrontier from %s to %s at %s", f.firstSupertime, f.lastSupertime, f.lastTime)
}

// newSeriesFrontier furnishes a populated diskFrontier for a given
// fingerprint.  If the series is absent, present will be false.
func newSeriesFrontier(f *clientmodel.Fingerprint, d *diskFrontier, i leveldb.Iterator) (s *seriesFrontier, present bool, err error) {
	lowerSeek := firstSupertime
	upperSeek := lastSupertime

	// If the diskFrontier for this iterator says that the candidate fingerprint
	// is outside of its seeking domain, there is no way that a seriesFrontier
	// could be materialized.  Simply bail.
	if !d.ContainsFingerprint(f) {
		return nil, false, nil
	}

	// If we are either the first or the last key in the database, we need to use
	// pessimistic boundary frontiers.
	if f.Equal(d.firstFingerprint) {
		lowerSeek = indexable.EncodeTime(d.firstSupertime)
	}
	if f.Equal(d.lastFingerprint) {
		upperSeek = indexable.EncodeTime(d.lastSupertime)
	}

	// TODO: Convert this to SampleKey.ToPartialDTO.
	fp := new(dto.Fingerprint)
	dumpFingerprint(fp, f)
	key := &dto.SampleKey{
		Fingerprint: fp,
		Timestamp:   upperSeek,
	}

	raw := coding.NewPBEncoder(key).MustEncode()
	i.Seek(raw)

	if i.Key() == nil {
		return nil, false, fmt.Errorf("illegal condition: empty key")
	}

	retrievedKey, err := extractSampleKey(i)
	if err != nil {
		panic(err)
	}

	retrievedFingerprint := retrievedKey.Fingerprint

	// The returned fingerprint may not match if the original seek key lives
	// outside of a metric's frontier.  This is probable, for we are seeking to
	// to the maximum allowed time, which could advance us to the next
	// fingerprint.
	//
	//
	if !retrievedFingerprint.Equal(f) {
		i.Previous()

		retrievedKey, err = extractSampleKey(i)
		if err != nil {
			panic(err)
		}
		retrievedFingerprint := retrievedKey.Fingerprint
		// If the previous key does not match, we know that the requested
		// fingerprint does not live in the database.
		if !retrievedFingerprint.Equal(f) {
			return nil, false, nil
		}
	}

	s = &seriesFrontier{
		lastSupertime: retrievedKey.FirstTimestamp,
		lastTime:      retrievedKey.LastTimestamp,
	}

	key.Timestamp = lowerSeek

	raw = coding.NewPBEncoder(key).MustEncode()

	i.Seek(raw)

	retrievedKey, err = extractSampleKey(i)
	if err != nil {
		panic(err)
	}

	retrievedFingerprint = retrievedKey.Fingerprint

	s.firstSupertime = retrievedKey.FirstTimestamp

	return s, true, nil
}

// Contains indicates whether a given time value is within the recorded
// interval.
func (s *seriesFrontier) Contains(t time.Time) bool {
	return !(t.Before(s.firstSupertime) || t.After(s.lastTime))
}

// InSafeSeekRange indicates whether the time is within the recorded time range
// and is safely seekable such that a seek does not result in an iterator point
// after the last value of the series or outside of the entire store.
func (s *seriesFrontier) InSafeSeekRange(t time.Time) (safe bool) {
	if !s.Contains(t) {
		return
	}

	if s.lastSupertime.Before(t) {
		return
	}

	return true
}

func (s *seriesFrontier) After(t time.Time) bool {
	return s.firstSupertime.After(t)
}
