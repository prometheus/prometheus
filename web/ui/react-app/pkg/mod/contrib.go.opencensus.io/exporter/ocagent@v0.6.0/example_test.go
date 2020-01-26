// Copyright 2018, OpenCensus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ocagent_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc/credentials"

	"contrib.go.opencensus.io/exporter/ocagent"
	"go.opencensus.io/trace"
)

func Example_insecure() {
	exp, err := ocagent.NewExporter(ocagent.WithInsecure(), ocagent.WithServiceName("engine"))
	if err != nil {
		log.Fatalf("Failed to create the agent exporter: %v", err)
	}
	defer exp.Stop()

	// Now register it as a trace exporter.
	trace.RegisterExporter(exp)

	// Then use the OpenCensus tracing library, like we normally would.
	ctx, span := trace.StartSpan(context.Background(), "AgentExporter-Example")
	defer span.End()

	for i := 0; i < 10; i++ {
		_, iSpan := trace.StartSpan(ctx, fmt.Sprintf("Sample-%d", i))
		<-time.After(6 * time.Millisecond)
		iSpan.End()
	}
}

func Example_withTLS() {
	// Please take at look at https://godoc.org/google.golang.org/grpc/credentials#TransportCredentials
	// for ways on how to initialize gRPC TransportCredentials.
	creds, err := credentials.NewClientTLSFromFile("my-cert.pem", "")
	if err != nil {
		log.Fatalf("Failed to create gRPC client TLS credentials: %v", err)
	}

	exp, err := ocagent.NewExporter(ocagent.WithTLSCredentials(creds), ocagent.WithServiceName("engine"))
	if err != nil {
		log.Fatalf("Failed to create the agent exporter: %v", err)
	}
	defer exp.Stop()

	// Now register it as a trace exporter.
	trace.RegisterExporter(exp)

	// Then use the OpenCensus tracing library, like we normally would.
	ctx, span := trace.StartSpan(context.Background(), "Securely-Talking-To-Agent-Span")
	defer span.End()

	for i := 0; i < 10; i++ {
		_, iSpan := trace.StartSpan(ctx, fmt.Sprintf("Sample-%d", i))
		<-time.After(6 * time.Millisecond)
		iSpan.End()
	}
}
