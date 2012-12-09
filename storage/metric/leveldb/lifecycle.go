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

package leveldb

import (
	index "github.com/matttproud/prometheus/storage/raw/index/leveldb"
	storage "github.com/matttproud/prometheus/storage/raw/leveldb"
	"io"
	"log"
)

type leveldbOpener func()

func (l *LevelDBMetricPersistence) Close() error {
	log.Printf("Closing LevelDBPersistence storage containers...")

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
				log.Printf("Closing LevelDBPersistence storage container: %s\n", name)
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

	log.Printf("Successfully closed all LevelDBPersistence storage containers.")

	return nil
}

func NewLevelDBMetricPersistence(baseDirectory string) (*LevelDBMetricPersistence, error) {
	log.Printf("Opening LevelDBPersistence storage containers...")

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
				emission.fingerprintToMetrics, err = storage.NewLevelDBPersistence(baseDirectory+"/label_name_and_value_pairs_by_fingerprint", 1000000, 10)
				errorChannel <- err
			},
		},
		{
			"Samples by Fingerprint",
			func() {
				var err error
				emission.metricSamples, err = storage.NewLevelDBPersistence(baseDirectory+"/samples_by_fingerprint", 1000000, 10)
				errorChannel <- err
			},
		},
		{
			"Fingerprints by Label Name",
			func() {
				var err error
				emission.labelNameToFingerprints, err = storage.NewLevelDBPersistence(baseDirectory+"/fingerprints_by_label_name", 1000000, 10)
				errorChannel <- err
			},
		},
		{
			"Fingerprints by Label Name and Value Pair",
			func() {
				var err error
				emission.labelSetToFingerprints, err = storage.NewLevelDBPersistence(baseDirectory+"/fingerprints_by_label_name_and_value_pair", 1000000, 10)
				errorChannel <- err
			},
		},
		{
			"Metric Membership Index",
			func() {
				var err error
				emission.metricMembershipIndex, err = index.NewLevelDBMembershipIndex(baseDirectory+"/metric_membership_index", 1000000, 10)
				errorChannel <- err
			},
		},
	}

	for _, subsystem := range subsystemOpeners {
		name := subsystem.name
		opener := subsystem.opener

		log.Printf("Opening LevelDBPersistence storage container: %s\n", name)

		go opener()
	}

	for i := 0; i < cap(errorChannel); i++ {
		openingError := <-errorChannel

		if openingError != nil {
			log.Printf("Could not open a LevelDBPersistence storage container: %q\n", openingError)

			return nil, openingError
		}
	}

	log.Printf("Successfully opened all LevelDBPersistence storage containers.\n")

	return emission, nil
}
