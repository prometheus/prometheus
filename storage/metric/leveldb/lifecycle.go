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

package leveldb

import (
	"flag"
	index "github.com/prometheus/prometheus/storage/raw/index/leveldb"
	storage "github.com/prometheus/prometheus/storage/raw/leveldb"
	"io"
	"log"
)

var (
	// These flag values are back of the envelope, though they seem sensible.
	// Please re-evaluate based on your own needs.
	fingerprintsToLabelPairCacheSize = flag.Int("fingerprintsToLabelPairCacheSizeBytes", 100*1024*1024, "The size for the fingerprint to label pair index (bytes).")
	samplesByFingerprintCacheSize    = flag.Int("samplesByFingerprintCacheSizeBytes", 500*1024*1024, "The size for the samples database (bytes).")
	labelNameToFingerprintsCacheSize = flag.Int("labelNameToFingerprintsCacheSizeBytes", 100*1024*1024, "The size for the label name to metric fingerprint index (bytes).")
	labelPairToFingerprintsCacheSize = flag.Int("labelPairToFingerprintsCacheSizeBytes", 100*1024*1024, "The size for the label pair to metric fingerprint index (bytes).")
	metricMembershipIndexCacheSize   = flag.Int("metricMembershipCacheSizeBytes", 50*1024*1024, "The size for the metric membership index (bytes).")
)

type leveldbOpener func()

func (l *LevelDBMetricPersistence) Close() error {
	var persistences = []struct {
		name   string
		closer io.Closer
	}{
		{
			"Fingerprint to Label Name and Value Pairs",
			l.fingerprintToMetrics,
		},
		{
			"Fingerprint Samples",
			l.metricSamples,
		},
		{
			"Label Name to Fingerprints",
			l.labelNameToFingerprints,
		},
		{
			"Label Name and Value Pairs to Fingerprints",
			l.labelSetToFingerprints,
		},
		{
			"Metric Membership Index",
			l.metricMembershipIndex,
		},
	}

	errorChannel := make(chan error, len(persistences))

	for _, persistence := range persistences {
		name := persistence.name
		closer := persistence.closer

		go func(name string, closer io.Closer) {
			if closer != nil {
				closingError := closer.Close()

				if closingError != nil {
					log.Printf("Could not close a LevelDBPersistence storage container; inconsistencies are possible: %q\n", closingError)
				}

				errorChannel <- closingError
			} else {
				errorChannel <- nil
			}
		}(name, closer)
	}

	for i := 0; i < cap(errorChannel); i++ {
		closingError := <-errorChannel

		if closingError != nil {
			return closingError
		}
	}

	return nil
}

func NewLevelDBMetricPersistence(baseDirectory string) (persistence *LevelDBMetricPersistence, err error) {
	errorChannel := make(chan error, 5)

	emission := &LevelDBMetricPersistence{}

	var subsystemOpeners = []struct {
		name   string
		opener leveldbOpener
	}{
		{
			"Label Names and Value Pairs by Fingerprint",
			func() {
				var err error
				emission.fingerprintToMetrics, err = storage.NewLevelDBPersistence(baseDirectory+"/label_name_and_value_pairs_by_fingerprint", *fingerprintsToLabelPairCacheSize, 10)
				errorChannel <- err
			},
		},
		{
			"Samples by Fingerprint",
			func() {
				var err error
				emission.metricSamples, err = storage.NewLevelDBPersistence(baseDirectory+"/samples_by_fingerprint", *samplesByFingerprintCacheSize, 10)
				errorChannel <- err
			},
		},
		{
			"Fingerprints by Label Name",
			func() {
				var err error
				emission.labelNameToFingerprints, err = storage.NewLevelDBPersistence(baseDirectory+"/fingerprints_by_label_name", *labelNameToFingerprintsCacheSize, 10)
				errorChannel <- err
			},
		},
		{
			"Fingerprints by Label Name and Value Pair",
			func() {
				var err error
				emission.labelSetToFingerprints, err = storage.NewLevelDBPersistence(baseDirectory+"/fingerprints_by_label_name_and_value_pair", *labelPairToFingerprintsCacheSize, 10)
				errorChannel <- err
			},
		},
		{
			"Metric Membership Index",
			func() {
				var err error
				emission.metricMembershipIndex, err = index.NewLevelDBMembershipIndex(baseDirectory+"/metric_membership_index", *metricMembershipIndexCacheSize, 10)
				errorChannel <- err
			},
		},
	}

	for _, subsystem := range subsystemOpeners {
		opener := subsystem.opener
		go opener()
	}

	for i := 0; i < cap(errorChannel); i++ {
		err = <-errorChannel

		if err != nil {
			log.Printf("Could not open a LevelDBPersistence storage container: %q\n", err)

			return
		}
	}
	persistence = emission

	return
}
