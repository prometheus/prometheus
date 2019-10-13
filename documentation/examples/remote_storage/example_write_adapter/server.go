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
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

func main() {
	http.HandleFunc("/read", func(w http.ResponseWriter, r *http.Request) {
		err := ReadHttpHandle(w, r)
		if err != nil {
			io.WriteString(w, err.Error())
			return
		}
	})

	http.HandleFunc("/labelvalues", func(writer http.ResponseWriter, request *http.Request) {
		lvs := make([]string, 0)
		lvs = append(lvs, "cpu")
		lvs = append(lvs, "memory")
		bytes, err := json.Marshal(lvs)
		if err != nil {
			io.WriteString(writer, err.Error())
		}
		fmt.Println("example result :", string(bytes))
		io.WriteString(writer, string(bytes))
	})

	port := 9876
	fmt.Println("port:", port)
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(port), nil))
}

func ReadHttpHandle(w http.ResponseWriter, r *http.Request) error {

	compressed, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		return err
	}

	var req prompb.ReadRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		return err
	}

	var resp *prompb.ReadResponse
	resp, err = Read(&req)
	if err != nil {
		return err
	}

	data, err := proto.Marshal(resp)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/x-protobuf")
	w.Header().Set("Content-Encoding", "snappy")

	compressed = snappy.Encode(nil, data)
	if _, err := w.Write(compressed); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}

	return nil
}

func Read(req *prompb.ReadRequest) (*prompb.ReadResponse, error) {
	resultResp := prompb.ReadResponse{
		Results: []*prompb.QueryResult{
			{Timeseries: make([]*prompb.TimeSeries, 0, len(req.Queries))},
		},
	}
	for _, query := range req.Queries {

		resultResp.Results = append(resultResp.Results, &prompb.QueryResult{
			Timeseries: []*prompb.TimeSeries{
				{
					Labels: []prompb.Label{
						{Name: "city", Value: "hangzhou"},
						{Name: "code", Value: "go"},
					},
					Samples: []prompb.Sample{
						{Timestamp: query.StartTimestampMs, Value: 123},
						{Timestamp: query.EndTimestampMs, Value: 456},
					},
				},
			},
		})

	}
	return &resultResp, nil

}
