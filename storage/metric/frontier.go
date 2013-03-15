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
	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/coding/indexable"
	"github.com/prometheus/prometheus/model"
	dto "github.com/prometheus/prometheus/model/generated"
	"time"
)

// diskFrontier describes an on-disk store of series to provide a
// representation of the known keyspace and time series values available.
//
// This is used to reduce the burden associated with LevelDB iterator
// management.
type diskFrontier struct {
	firstFingerprint model.Fingerprint
	firstSupertime   time.Time
	lastFingerprint  model.Fingerprint
	lastSupertime    time.Time
}

func (f *diskFrontier) String() string {
	return fmt.Sprintf("diskFrontier from %s at %s to %s at %s", f.firstFingerprint.ToRowKey(), f.firstSupertime, f.lastFingerprint.ToRowKey(), f.lastSupertime)
}

func (f *diskFrontier) ContainsFingerprint(fingerprint model.Fingerprint) bool {
	return !(fingerprint.Less(f.firstFingerprint) || f.lastFingerprint.Less(fingerprint))
}

func newDiskFrontier(i iterator) (d *diskFrontier, err error) {
	i.SeekToLast()
	if i.Key() == nil {
		return
	}
	lastKey, err := extractSampleKey(i)
	if err != nil {
		panic(err)
	}

	i.SeekToFirst()
	firstKey, err := extractSampleKey(i)
	if i.Key() == nil {
		return
	}
	if err != nil {
		panic(err)
	}

	d = &diskFrontier{}

	d.firstFingerprint = model.NewFingerprintFromRowKey(*firstKey.Fingerprint.Signature)
	d.firstSupertime = indexable.DecodeTime(firstKey.Timestamp)
	d.lastFingerprint = model.NewFingerprintFromRowKey(*lastKey.Fingerprint.Signature)
	d.lastSupertime = indexable.DecodeTime(lastKey.Timestamp)

	return
}

// seriesFrontier represents the valid seek frontier for a given series.
type seriesFrontier struct {
	firstSupertime time.Time
	lastSupertime  time.Time
	lastTime       time.Time
}

func (f seriesFrontier) String() string {
	return fmt.Sprintf("seriesFrontier from %s to %s at %s", f.firstSupertime, f.lastSupertime, f.lastTime)
}

// newSeriesFrontier furnishes a populated diskFrontier for a given
// fingerprint.  A nil diskFrontier will be returned if the series cannot
// be found in the store.
func newSeriesFrontier(f model.Fingerprint, d diskFrontier, i iterator) (s *seriesFrontier, err error) {
	var (
		lowerSeek = firstSupertime
		upperSeek = lastSupertime
	)

	// If we are either the first or the last key in the database, we need to use
	// pessimistic boundary frontiers.
	if f.Equal(d.firstFingerprint) {
		lowerSeek = indexable.EncodeTime(d.firstSupertime)
	}
	if f.Equal(d.lastFingerprint) {
		upperSeek = indexable.EncodeTime(d.lastSupertime)
	}

	key := &dto.SampleKey{
		Fingerprint: f.ToDTO(),
		Timestamp:   upperSeek,
	}

	raw, err := coding.NewProtocolBufferEncoder(key).Encode()
	if err != nil {
		panic(err)
	}
	i.Seek(raw)

	if i.Key() == nil {
		return
	}

	retrievedKey, err := extractSampleKey(i)
	if err != nil {
		panic(err)
	}

	retrievedFingerprint := model.NewFingerprintFromRowKey(*retrievedKey.Fingerprint.Signature)

	// The returned fingerprint may not match if the original seek key lives
	// outside of a metric's frontier.  This is probable, for we are seeking to
	// to the maximum allowed time, which could advance us to the next
	// fingerprint.
	//
	//
	if !retrievedFingerprint.Equal(f) {
		i.Prev()

		retrievedKey, err = extractSampleKey(i)
		if err != nil {
			panic(err)
		}
		retrievedFingerprint := model.NewFingerprintFromRowKey(*retrievedKey.Fingerprint.Signature)
		// If the previous key does not match, we know that the requested
		// fingerprint does not live in the database.
		if !retrievedFingerprint.Equal(f) {
			return
		}
	}

	s = &seriesFrontier{
		lastSupertime: indexable.DecodeTime(retrievedKey.Timestamp),
		lastTime:      time.Unix(*retrievedKey.LastTimestamp, 0),
	}

	key.Timestamp = lowerSeek

	raw, err = coding.NewProtocolBufferEncoder(key).Encode()
	if err != nil {
		panic(err)
	}

	i.Seek(raw)

	retrievedKey, err = extractSampleKey(i)
	if err != nil {
		panic(err)
	}

	retrievedFingerprint = model.NewFingerprintFromRowKey(*retrievedKey.Fingerprint.Signature)

	s.firstSupertime = indexable.DecodeTime(retrievedKey.Timestamp)

	return
}
