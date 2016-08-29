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
	"io"
	"io/ioutil"
	"log"
	"net"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage/remote"
)

type server struct{}

func (server *server) Write(ctx context.Context, req *remote.WriteRequest) (*remote.WriteResponse, error) {
	for _, ts := range req.Timeseries {
		m := make(model.Metric, len(ts.Labels))
		for _, l := range ts.Labels {
			m[model.LabelName(l.Name)] = model.LabelValue(l.Value)
		}
		fmt.Println(m)

		for _, s := range ts.Samples {
			fmt.Printf("  %f %d\n", s.Value, s.TimestampMs)
		}
	}

	return &remote.WriteResponse{}, nil
}

type snappyDecompressor struct{}

func (d *snappyDecompressor) Do(r io.Reader) ([]byte, error) {
	sr := snappy.NewReader(r)
	return ioutil.ReadAll(sr)
}

func (d *snappyDecompressor) Type() string {
	return "snappy"
}

func main() {
	lis, err := net.Listen("tcp", ":1234")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	s := grpc.NewServer(grpc.RPCDecompressor(&snappyDecompressor{}))
	remote.RegisterWriteServer(s, &server{})
	s.Serve(lis)
}
