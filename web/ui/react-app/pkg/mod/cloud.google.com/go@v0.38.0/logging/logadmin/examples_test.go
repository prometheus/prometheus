// Copyright 2016 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logadmin_test

import (
	"context"
	"fmt"

	"cloud.google.com/go/logging/logadmin"
)

func ExampleNewClient() {
	ctx := context.Background()
	client, err := logadmin.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	// Use client to manage logs, metrics and sinks.
	// Close the client when finished.
	if err := client.Close(); err != nil {
		// TODO: Handle error.
	}
}

func ExampleClient_DeleteLog() {
	ctx := context.Background()
	client, err := logadmin.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	err = client.DeleteLog(ctx, "my-log")
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleClient_CreateMetric() {
	ctx := context.Background()
	client, err := logadmin.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	err = client.CreateMetric(ctx, &logadmin.Metric{
		ID:          "severe-errors",
		Description: "entries at ERROR or higher severities",
		Filter:      "severity >= ERROR",
	})
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleClient_DeleteMetric() {
	ctx := context.Background()
	client, err := logadmin.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	if err := client.DeleteMetric(ctx, "severe-errors"); err != nil {
		// TODO: Handle error.
	}
}

func ExampleClient_Metric() {
	ctx := context.Background()
	client, err := logadmin.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	m, err := client.Metric(ctx, "severe-errors")
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(m)
}

func ExampleClient_UpdateMetric() {
	ctx := context.Background()
	client, err := logadmin.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	err = client.UpdateMetric(ctx, &logadmin.Metric{
		ID:          "severe-errors",
		Description: "entries at high severities",
		Filter:      "severity > ERROR",
	})
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleClient_CreateSink() {
	ctx := context.Background()
	client, err := logadmin.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	sink, err := client.CreateSink(ctx, &logadmin.Sink{
		ID:          "severe-errors-to-gcs",
		Destination: "storage.googleapis.com/my-bucket",
		Filter:      "severity >= ERROR",
	})
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(sink)
}

func ExampleClient_DeleteSink() {
	ctx := context.Background()
	client, err := logadmin.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	if err := client.DeleteSink(ctx, "severe-errors-to-gcs"); err != nil {
		// TODO: Handle error.
	}
}

func ExampleClient_Sink() {
	ctx := context.Background()
	client, err := logadmin.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	s, err := client.Sink(ctx, "severe-errors-to-gcs")
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(s)
}

func ExampleClient_UpdateSink() {
	ctx := context.Background()
	client, err := logadmin.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	sink, err := client.UpdateSink(ctx, &logadmin.Sink{
		ID:          "severe-errors-to-gcs",
		Destination: "storage.googleapis.com/my-other-bucket",
		Filter:      "severity >= ERROR",
	})
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(sink)
}
