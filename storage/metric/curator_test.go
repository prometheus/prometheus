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
	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/raw"
	"sort"
	"testing"
	"time"
)

type (
	keyPair struct {
		fingerprint model.Fingerprint
		time        time.Time
	}

	fakeCurationStates map[model.Fingerprint]time.Time
	fakeSamples        map[keyPair][]float32
	fakeWatermarks     map[model.Fingerprint]time.Time

	in struct {
		curationStates fakeCurationStates
		samples        fakeSamples
		watermarks     fakeWatermarks
		cutOff         time.Time
		grouping       uint32
	}

	out struct {
		curationStates fakeCurationStates
		samples        fakeSamples
		watermarks     fakeWatermarks
	}
)

func (c fakeCurationStates) Has(_ coding.Encoder) (bool, error) {
	panic("unimplemented")
}

func (c fakeCurationStates) Get(_ coding.Encoder) ([]byte, error) {
	panic("unimplemented")
}

func (c fakeCurationStates) Drop(_ coding.Encoder) error {
	panic("unimplemented")
}

func (c fakeCurationStates) Put(_, _ coding.Encoder) error {
	panic("unimplemented")
}

func (c fakeCurationStates) Close() error {
	panic("unimplemented")
}

func (c fakeCurationStates) ForEach(d storage.RecordDecoder, f storage.RecordFilter, o storage.RecordOperator) (scannedAll bool, err error) {
	var (
		fingerprints model.Fingerprints
	)

	for f := range c {
		fingerprints = append(fingerprints, f)
	}

	sort.Sort(fingerprints)

	for _, k := range fingerprints {
		v := c[k]

		var (
			decodedKey   interface{}
			decodedValue interface{}
		)

		decodedKey, err = d.DecodeKey(k)
		if err != nil {
			continue
		}

		decodedValue, err = d.DecodeValue(v)
		if err != nil {
			continue
		}

		switch f.Filter(decodedKey, decodedValue) {
		case storage.STOP:
			return
		case storage.SKIP:
			continue
		case storage.ACCEPT:
			opErr := o.Operate(decodedKey, decodedValue)
			if opErr != nil {
				if opErr.Continuable {
					continue
				}
				break
			}
		}
	}

	return
}

func (c fakeCurationStates) Commit(_ raw.Batch) error {
	panic("unimplemented")
}

func (c fakeSamples) Has(_ coding.Encoder) (bool, error) {
	panic("unimplemented")
}

func (c fakeSamples) Get(_ coding.Encoder) ([]byte, error) {
	panic("unimplemented")
}

func (c fakeSamples) Drop(_ coding.Encoder) error {
	panic("unimplemented")
}

func (c fakeSamples) Put(_, _ coding.Encoder) error {
	panic("unimplemented")
}

func (c fakeSamples) Close() error {
	panic("unimplemented")
}

func (c fakeSamples) ForEach(d storage.RecordDecoder, f storage.RecordFilter, o storage.RecordOperator) (scannedAll bool, err error) {
	panic("unimplemented")
}

func (c fakeSamples) Commit(_ raw.Batch) (err error) {
	panic("unimplemented")
}

func (c fakeWatermarks) Has(_ coding.Encoder) (bool, error) {
	panic("unimplemented")
}

func (c fakeWatermarks) Get(_ coding.Encoder) ([]byte, error) {
	panic("unimplemented")
}

func (c fakeWatermarks) Drop(_ coding.Encoder) error {
	panic("unimplemented")
}

func (c fakeWatermarks) Put(_, _ coding.Encoder) error {
	panic("unimplemented")
}

func (c fakeWatermarks) Close() error {
	panic("unimplemented")
}

func (c fakeWatermarks) ForEach(d storage.RecordDecoder, f storage.RecordFilter, o storage.RecordOperator) (scannedAll bool, err error) {
	panic("unimplemented")
}

func (c fakeWatermarks) Commit(_ raw.Batch) (err error) {
	panic("unimplemented")
}

func TestCurator(t *testing.T) {
	var (
		scenarios = []struct {
			in  in
			out out
		}{
			{
				in: in{
					curationStates: fakeCurationStates{
						model.NewFingerprintFromRowKey("0-A-10-Z"): testInstant.Add(5 * time.Minute),
						model.NewFingerprintFromRowKey("1-B-10-A"): testInstant.Add(4 * time.Minute),
					},
					watermarks: fakeWatermarks{},
					samples:    fakeSamples{},
					cutOff:     testInstant.Add(5 * time.Minute),
					grouping:   5,
				},
			},
		}
	)

	for _, scenario := range scenarios {
		var (
			in = scenario.in

			curationStates = in.curationStates
			samples        = in.samples
			watermarks     = in.watermarks
			cutOff         = in.cutOff
			grouping       = in.grouping
		)

		_ = newCurator(cutOff, grouping, curationStates, samples, watermarks)
	}
}
