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

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/storage/remote"
)

func main() {
	http.HandleFunc("/receive", func(w http.ResponseWriter, r *http.Request) {
		enc := r.Header.Get("Content-Encoding")
		if enc == "" {
			http.Error(w, "missing Content-Encoding header", http.StatusUnsupportedMediaType)
			return
		}
		if enc != "snappy" {
			http.Error(w, "unknown encoding, only snappy supported", http.StatusUnsupportedMediaType)
			return
		}

		contentType := r.Header.Get("Content-Type")
		if contentType == "" {
			http.Error(w, "missing Content-Type header", http.StatusUnsupportedMediaType)
		}

		defer func() { _ = r.Body.Close() }()

		// Very simplistic content parsing, see
		// storage/remote/write_handler.go#WriteHandler.ServeHTTP for production example.
		switch contentType {
		case "application/x-protobuf", "application/x-protobuf;proto=prometheus.WriteRequest":
			req, err := remote.DecodeWriteRequest(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			printV1(req)
		case "application/x-protobuf;proto=io.prometheus.write.v2.Request":
			req, err := remote.DecodeWriteV2Request(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			err = printV2(req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		default:
			msg := fmt.Sprintf("Unknown remote write content type: %s", contentType)
			fmt.Println(msg)
			http.Error(w, msg, http.StatusBadRequest)
		}
	})
	log.Fatal(http.ListenAndServe(":1234", nil))
}

func printV1(req *prompb.WriteRequest) {
	b := labels.NewScratchBuilder(0)
	for _, ts := range req.Timeseries {
		fmt.Println(ts.ToLabels(&b, nil))

		for _, s := range ts.Samples {
			fmt.Printf("\tSample:  %f %d\n", s.Value, s.Timestamp)
		}
		for _, ep := range ts.Exemplars {
			e := ep.ToExemplar(&b, nil)
			fmt.Printf("\tExemplar:  %+v %f %d\n", e.Labels, e.Value, ep.Timestamp)
		}
		for _, hp := range ts.Histograms {
			if hp.IsFloatHistogram() {
				h := hp.ToFloatHistogram()
				fmt.Printf("\tHistogram:  %s\n", h.String())
				continue
			}
			h := hp.ToIntHistogram()
			fmt.Printf("\tHistogram:  %s\n", h.String())
		}
	}
}

func printV2(req *writev2.Request) error {
	b := labels.NewScratchBuilder(0)
	for _, ts := range req.Timeseries {
		l, err := ts.ToLabels(&b, req.Symbols)
		if err != nil {
			return err
		}
		m := ts.ToMetadata(req.Symbols)
		fmt.Println(l, m)

		for _, s := range ts.Samples {
			fmt.Printf("\tSample:  %f %d\n", s.Value, s.Timestamp)
		}
		for _, ep := range ts.Exemplars {
			e, err := ep.ToExemplar(&b, req.Symbols)
			if err != nil {
				return err
			}
			fmt.Printf("\tExemplar:  %+v %f %d\n", e.Labels, e.Value, ep.Timestamp)
		}
		for _, hp := range ts.Histograms {
			if hp.IsFloatHistogram() {
				h := hp.ToFloatHistogram()
				fmt.Printf("\tHistogram:  %s\n", h.String())
				continue
			}
			h := hp.ToIntHistogram()
			fmt.Printf("\tHistogram:  %s\n", h.String())
		}
	}
	return nil
}
