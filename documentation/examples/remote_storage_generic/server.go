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
	"net"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/prometheus/prometheus/storage/remote/generic"
)

type server struct{}

func (server *server) Write(ctx context.Context, req *generic.GenericWriteRequest) (*generic.GenericWriteResponse, error) {
	for _, ts := range req.Timeseries {
		fmt.Printf("%s", ts.Name)
		for _, l := range ts.Labels {
			fmt.Printf(" %s=%s", l.Name, l.Value)
		}
		fmt.Printf("\n")

		for _, s := range ts.Samples {
			fmt.Printf("  %f %d\n", s.Value, s.TimestampMs)
		}
	}

	return &generic.GenericWriteResponse{}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":1234")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	generic.RegisterGenericWriteServer(s, &server{})
	s.Serve(lis)
}
