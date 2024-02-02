// Copyright 2016 The Prometheus Authors
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

package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/storage/remote"
)

func main() {
	http.HandleFunc("/receive", func(w http.ResponseWriter, r *http.Request) {
		req, err := remote.DecodeWriteRequest(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		for _, ts := range req.Timeseries {
			m := make(model.Metric, len(ts.Labels))
			for _, l := range ts.Labels {
				m[model.LabelName(l.Name)] = model.LabelValue(l.Value)
			}
			fmt.Println(m)

			for _, s := range ts.Samples {
				fmt.Printf("\tSample:  %f %d\n", s.Value, s.Timestamp)
			}

			for _, e := range ts.Exemplars {
				m := make(model.Metric, len(e.Labels))
				for _, l := range e.Labels {
					m[model.LabelName(l.Name)] = model.LabelValue(l.Value)
				}
				fmt.Printf("\tExemplar:  %+v %f %d\n", m, e.Value, e.Timestamp)
			}

			for _, hp := range ts.Histograms {
				h := remote.HistogramProtoToHistogram(hp)
				fmt.Printf("\tHistogram:  %s\n", h.String())
			}
		}
	})

	log.Fatal(http.ListenAndServe(":1234", nil))
}
