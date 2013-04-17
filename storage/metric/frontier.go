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
	"code.google.com/p/goprotobuf/proto"
	"fmt"
	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/model"
	dto "github.com/prometheus/prometheus/model/generated"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
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

func (f diskFrontier) String() string {
	return fmt.Sprintf("diskFrontier from %s at %s to %s at %s", f.firstFingerprint.ToRowKey(), f.firstSupertime, f.lastFingerprint.ToRowKey(), f.lastSupertime)
}

func (f diskFrontier) ContainsFingerprint(fingerprint model.Fingerprint) bool {
	return !(fingerprint.Less(f.firstFingerprint) || f.lastFingerprint.Less(fingerprint))
}

func newDiskFrontier(i leveldb.Iterator) (d *diskFrontier, err error) {

	if !i.SeekToLast() || i.Key() == nil {
		return
	}

	lastKey, err := extractSampleKey(i)
	if err != nil {
		panic(err)
	}

	if !i.SeekToFirst() || i.Key() == nil {
		return
	}
	firstKey, err := extractSampleKey(i)
	if i.Key() == nil {
		return
	}
	if err != nil {
		panic(err)
	}

	d = &diskFrontier{}

	d.firstFingerprint = model.NewFingerprintFromDTO(firstKey.Fingerprint)
	d.firstSupertime = time.Unix(*firstKey.Timestamp, 0)
	d.lastFingerprint = model.NewFingerprintFromDTO(lastKey.Fingerprint)
	d.lastSupertime = time.Unix(*lastKey.Timestamp, 0)

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
func newSeriesFrontier(f model.Fingerprint, d diskFrontier, i leveldb.Iterator) (s *seriesFrontier, err error) {
	var (
		lowerSeek = firstSupertime
		upperSeek = lastSupertime
	)

	// If the diskFrontier for this iterator says that the candidate fingerprint
	// is outside of its seeking domain, there is no way that a seriesFrontier
	// could be materialized.  Simply bail.
	if !d.ContainsFingerprint(f) {
		return
	}

	// If we are either the first or the last key in the database, we need to use
	// pessimistic boundary frontiers.
	if f.Equal(d.firstFingerprint) {
		lowerSeek = d.firstSupertime.Unix()
	}
	if f.Equal(d.lastFingerprint) {
		upperSeek = d.lastSupertime.Unix()
	}

	key := &dto.SampleKey{
		Fingerprint: f.ToDTO(),
		Timestamp:   proto.Int64(upperSeek),
	}

	raw, err := coding.NewProtocolBuffer(key).Encode()
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

	retrievedFingerprint := model.NewFingerprintFromDTO(retrievedKey.Fingerprint)

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
		retrievedFingerprint := model.NewFingerprintFromDTO(retrievedKey.Fingerprint)
		// If the previous key does not match, we know that the requested
		// fingerprint does not live in the database.
		if !retrievedFingerprint.Equal(f) {
			return
		}
	}

	s = &seriesFrontier{
		lastSupertime: time.Unix(*retrievedKey.Timestamp, 0),
		lastTime:      time.Unix(*retrievedKey.LastTimestamp, 0),
	}

	key.Timestamp = proto.Int64(lowerSeek)

	raw, err = coding.NewProtocolBuffer(key).Encode()
	if err != nil {
		panic(err)
	}

	i.Seek(raw)

	retrievedKey, err = extractSampleKey(i)
	if err != nil {
		panic(err)
	}

	retrievedFingerprint = model.NewFingerprintFromDTO(retrievedKey.Fingerprint)

	s.firstSupertime = time.Unix(*retrievedKey.Timestamp, 0)

	return
}
