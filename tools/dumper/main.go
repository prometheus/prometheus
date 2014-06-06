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

/*
import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/golang/glog"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/storage/metric/tiered"
)

var (
	storageRoot   = flag.String("storage.root", "", "The path to the storage root for Prometheus.")
	dieOnBadChunk = flag.Bool("dieOnBadChunk", false, "Whether to die upon encountering a bad chunk.")
)

type SamplesDumper struct {
	*csv.Writer
}

func (d *SamplesDumper) Operate(key, value interface{}) *storage.OperatorError {
	sampleKey := key.(*tiered.SampleKey)
	if *dieOnBadChunk && sampleKey.FirstTimestamp.After(sampleKey.LastTimestamp) {
		glog.Fatalf("Chunk: First time (%v) after last time (%v): %v\n", sampleKey.FirstTimestamp.Unix(), sampleKey.LastTimestamp.Unix(), sampleKey)
	}
	for i, sample := range value.(metric.Values) {
		if *dieOnBadChunk && (sample.Timestamp.Before(sampleKey.FirstTimestamp) || sample.Timestamp.After(sampleKey.LastTimestamp)) {
			glog.Fatalf("Sample not within chunk boundaries: chunk FirstTimestamp (%v), chunk LastTimestamp (%v) vs. sample Timestamp (%v)\n", sampleKey.FirstTimestamp.Unix(), sampleKey.LastTimestamp.Unix(), sample.Timestamp)
		}
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
				Error:       err,
				Continuable: false,
			}
		}
	}
	return nil
}

func main() {
	flag.Parse()

	if storageRoot == nil || *storageRoot == "" {
		glog.Fatal("Must provide a path...")
	}

	persistence, err := tiered.NewLevelDBPersistence(*storageRoot)
	if err != nil {
		glog.Fatal(err)
	}
	defer persistence.Close()

	dumper := &SamplesDumper{
		csv.NewWriter(os.Stdout),
	}

	entire, err := persistence.MetricSamples.ForEach(&tiered.MetricSamplesDecoder{}, &tiered.AcceptAllFilter{}, dumper)
	if err != nil {
		glog.Fatal("Error dumping samples: ", err)
	}
	if !entire {
		glog.Fatal("Didn't scan entire corpus")
	}
	dumper.Flush()
	if err = dumper.Error(); err != nil {
		glog.Fatal("Error flushing CSV: ", err)
	}
}
*/

func main() {
}
