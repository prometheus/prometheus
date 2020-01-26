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

package logging_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"cloud.google.com/go/logging"
	"go.opencensus.io/trace"
)

func ExampleNewClient() {
	ctx := context.Background()
	client, err := logging.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	// Use client to manage logs, metrics and sinks.
	// Close the client when finished.
	if err := client.Close(); err != nil {
		// TODO: Handle error.
	}
}

func ExampleClient_Ping() {
	ctx := context.Background()
	client, err := logging.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	if err := client.Ping(ctx); err != nil {
		// TODO: Handle error.
	}
}

// Although Logger.Flush and Client.Close both return errors, they don't tell you
// whether the errors were frequent or significant. For most programs, it doesn't
// matter if there were a few errors while writing logs, although if those few errors
// indicated a bug in your program, you might want to know about them. The best way
// to handle errors is by setting the OnError function. If it runs quickly, it will
// see every error generated during logging.
func ExampleNewClient_errorFunc() {
	ctx := context.Background()
	client, err := logging.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	// Print all errors to stdout, and count them. Multiple calls to the OnError
	// function never happen concurrently, so there is no need for locking nErrs,
	// provided you don't read it until after the logging client is closed.
	var nErrs int
	client.OnError = func(e error) {
		fmt.Fprintf(os.Stdout, "logging: %v", e)
		nErrs++
	}
	// Use client to manage logs, metrics and sinks.
	// Close the client when finished.
	if err := client.Close(); err != nil {
		// TODO: Handle error.
	}
	fmt.Printf("saw %d errors\n", nErrs)
}

func ExampleClient_Logger() {
	ctx := context.Background()
	client, err := logging.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	lg := client.Logger("my-log")
	_ = lg // TODO: use the Logger.
}

func ExampleLogger_LogSync() {
	ctx := context.Background()
	client, err := logging.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	lg := client.Logger("my-log")
	err = lg.LogSync(ctx, logging.Entry{Payload: "red alert"})
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleLogger_Log() {
	ctx := context.Background()
	client, err := logging.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	lg := client.Logger("my-log")
	lg.Log(logging.Entry{Payload: "something happened"})
}

// An Entry payload can be anything that marshals to a
// JSON object, like a struct.
func ExampleLogger_Log_struct() {
	type MyEntry struct {
		Name  string
		Count int
	}

	ctx := context.Background()
	client, err := logging.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	lg := client.Logger("my-log")
	lg.Log(logging.Entry{Payload: MyEntry{Name: "Bob", Count: 3}})
}

// To log a JSON value, wrap it in json.RawMessage.
func ExampleLogger_Log_json() {
	ctx := context.Background()
	client, err := logging.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	lg := client.Logger("my-log")
	j := []byte(`{"Name": "Bob", "Count": 3}`)
	lg.Log(logging.Entry{Payload: json.RawMessage(j)})
}

func ExampleLogger_Flush() {
	ctx := context.Background()
	client, err := logging.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	lg := client.Logger("my-log")
	lg.Log(logging.Entry{Payload: "something happened"})
	lg.Flush()
}

func ExampleLogger_StandardLogger() {
	ctx := context.Background()
	client, err := logging.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	lg := client.Logger("my-log")
	slg := lg.StandardLogger(logging.Info)
	slg.Println("an informative message")
}

func ExampleParseSeverity() {
	sev := logging.ParseSeverity("ALERT")
	fmt.Println(sev)
	// Output: Alert
}

// This example shows how to create a Logger that disables OpenCensus tracing of the
// WriteLogEntries RPC.
func ExampleContextFunc() {
	ctx := context.Background()
	client, err := logging.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	lg := client.Logger("logID", logging.ContextFunc(func() (context.Context, func()) {
		ctx, span := trace.StartSpan(context.Background(), "this span will not be exported",
			trace.WithSampler(trace.NeverSample()))
		return ctx, span.End
	}))
	_ = lg // TODO: Use lg
}
