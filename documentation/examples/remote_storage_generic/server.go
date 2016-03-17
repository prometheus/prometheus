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
	"bytes"
	"fmt"
	"log"
	"net/http"

	"github.com/golang/protobuf/proto"

	"github.com/prometheus/prometheus/storage/remote/generic"
)

func main() {
	http.HandleFunc("/remote", func(w http.ResponseWriter, r *http.Request) {
		req := &generic.GenericWriteRequest{}
		buf := bytes.Buffer{}
		_, err := buf.ReadFrom(r.Body)
		if err != nil {
			log.Print(err)
			return
		}
		err = proto.Unmarshal(buf.Bytes(), req)
		if err != nil {
			log.Print(err)
			return
		}

		for _, ts := range req.Timeseries {
			fmt.Printf("%s", ts.GetName())
			for _, l := range ts.Labels {
				fmt.Printf(" %s=%s", l.GetName(), l.GetValue())
			}
			fmt.Printf("\n")

			for _, s := range ts.Samples {
				fmt.Printf("  %f %d\n", s.GetValue(), s.GetTimestampMs())
			}
		}
	})

	log.Fatal(http.ListenAndServe(":1234", nil))
}
