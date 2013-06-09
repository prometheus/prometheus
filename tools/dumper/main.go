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

// Dumper is responsible for dumping all samples along with metadata contained
// in a given Prometheus metrics storage. It prints samples in unquoted CSV
// format, with commas as field separators:
//
// <fingerprint>,<chunk_first_time>,<chunk_last_time>,<chunk_sample_count>,<chunk_index>,<timestamp>,<value>
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"code.google.com/p/goprotobuf/proto"

	dto "github.com/prometheus/prometheus/model/generated"

	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/metric"
)

var (
	storageRoot = flag.String("storage.root", "", "The path to the storage root for Prometheus.")
)

type SamplesDumper struct {
	*csv.Writer
}

func (d SamplesDumper) DecodeKey(in interface{}) (interface{}, error) {
	key := &dto.SampleKey{}
	err := proto.Unmarshal(in.([]byte), key)
	if err != nil {
		return nil, err
	}

	return model.NewSampleKeyFromDTO(key), nil
}

func (d SamplesDumper) DecodeValue(in interface{}) (interface{}, error) {
	values := &dto.SampleValueSeries{}
	err := proto.Unmarshal(in.([]byte), values)
	if err != nil {
		return nil, err
	}

	return model.NewValuesFromDTO(values), nil
}

func (d SamplesDumper) Filter(_, _ interface{}) storage.FilterResult {
	return storage.ACCEPT
}

func (d SamplesDumper) Operate(key, value interface{}) *storage.OperatorError {
	sampleKey := key.(model.SampleKey)
	for i, sample := range value.(model.Values) {
		d.Write([]string{
			sampleKey.Fingerprint.String(),
			strconv.FormatInt(sampleKey.FirstTimestamp.Unix(), 10),
			strconv.FormatInt(sampleKey.LastTimestamp.Unix(), 10),
			strconv.FormatUint(uint64(sampleKey.SampleCount), 10),
			strconv.Itoa(i),
			strconv.FormatInt(sample.Timestamp.Unix(), 10),
			fmt.Sprintf("%v", sample.Value),
		})
		if err := d.Error(); err != nil {
			return &storage.OperatorError{
				error:       err,
				Continuable: false,
			}
		}
	}
	return nil
}

func main() {
	flag.Parse()

	if storageRoot == nil || *storageRoot == "" {
		log.Fatal("Must provide a path...")
	}

	persistence, err := metric.NewLevelDBMetricPersistence(*storageRoot)
	if err != nil {
		log.Fatal(err)
	}
	defer persistence.Close()

	dumper := SamplesDumper{csv.NewWriter(os.Stdout)}
	entire, err := persistence.MetricSamples.ForEach(dumper, dumper, dumper)
	if err != nil {
		log.Fatalf("Error dumping samples: %s", err)
	}
	if !entire {
		log.Fatalf("Didn't scan entire corpus")
	}
	dumper.Flush()
	if err = dumper.Error(); err != nil {
		log.Fatalf("Error flushing CSV: %s", err)
	}
}
