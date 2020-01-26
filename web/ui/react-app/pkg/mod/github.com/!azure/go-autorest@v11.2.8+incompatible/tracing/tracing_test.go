package tracing

// Copyright 2018 Microsoft Corporation
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"testing"

	"contrib.go.opencensus.io/exporter/ocagent"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
)

func TestNoTracingByDefault(t *testing.T) {
	if expected, got := false, IsEnabled(); expected != got {
		t.Fatalf("By default expected %t, got %t", expected, got)
	}

	if sampler == nil {
		t.Fatal("By default expected non nil sampler")
	}

	if Transport.GetStartOptions(&http.Request{}).Sampler == nil {
		t.Fatalf("By default expected configured Sampler to be non-nil")
	}

	for n := range views {
		v := view.Find(n)
		if v != nil {
			t.Fatalf("By default expected no registered views, found %s", v.Name)
		}
	}
}

func TestEnableTracing(t *testing.T) {
	err := Enable()

	if err != nil {
		t.Fatalf("Enable failed, got error %v", err)
	}
	if !IsEnabled() {
		t.Fatalf("Enable failed, IsEnabled() is %t", IsEnabled())
	}
	if sampler != nil {
		t.Fatalf("Enable failed, expected nil sampler, got %v", sampler)
	}

	if Transport.GetStartOptions(&http.Request{}).Sampler != nil {
		t.Fatalf("Enable failed, expected Transport.GetStartOptions.Sampler to be nil")
	}

	for n, v := range views {
		fv := view.Find(n)
		if fv == nil || !reflect.DeepEqual(v, fv) {
			t.Fatalf("Enable failed, view %s was not registered", n)
		}
	}
}

func TestTracingByEnv(t *testing.T) {
	os.Setenv("AZURE_SDK_TRACING_ENABLED", "")
	enableFromEnv()
	if !IsEnabled() {
		t.Fatalf("Enable failed, IsEnabled() is %t", IsEnabled())
	}
	if sampler != nil {
		t.Fatalf("Enable failed, expected nil sampler, got %v", sampler)
	}

	if Transport.GetStartOptions(&http.Request{}).Sampler != nil {
		t.Fatalf("Enable failed, expected Transport.GetStartOptions.Sampler to be nil")
	}

	for n, v := range views {
		fv := view.Find(n)
		if fv == nil || !reflect.DeepEqual(v, fv) {
			t.Fatalf("Enable failed, view %s was not registered", n)
		}
	}
}

func TestEnableTracingWithAIError(t *testing.T) {
	agentEndpoint := fmt.Sprintf("%s:%d", ocagent.DefaultAgentHost, ocagent.DefaultAgentPort)
	err := EnableWithAIForwarding(agentEndpoint)
	if !IsEnabled() {
		t.Fatalf("Enable failed, IsEnabled() is %t", IsEnabled())
	}
	if sampler != nil {
		t.Fatalf("Enable failed, expected nil sampler, got %v", sampler)
	}

	if Transport.GetStartOptions(&http.Request{}).Sampler != nil {
		t.Fatalf("Enable failed, expected Transport.GetStartOptions.Sampler to be nil")
	}

	for n, v := range views {
		fv := view.Find(n)
		if fv == nil || !reflect.DeepEqual(v, fv) {
			t.Fatalf("Enable failed, view %s was not registered", n)
		}
	}

	if err == nil {
		t.Fatal("Expected error on no agent running, got nil")
	}
}

func TestDisableTracing(t *testing.T) {
	Enable()
	Disable()
	if expected, got := false, IsEnabled(); expected != got {
		t.Fatalf("By default expected %t, got %t", expected, got)
	}

	if sampler == nil {
		t.Fatal("By default expected non nil sampler")
	}

	if Transport.GetStartOptions(&http.Request{}).Sampler == nil {
		t.Fatalf("By default expected configured Sampler to be non-nil")
	}

	for n := range views {
		v := view.Find(n)
		if v != nil {
			t.Fatalf("By default expected no registered views, found %s", v.Name)
		}
	}
}

func TestStartSpan(t *testing.T) {
	ctx := StartSpan(context.Background(), "testSpan")
	defer EndSpan(ctx, 200, nil)

	span := trace.FromContext(ctx)
	if span == nil {
		t.Fatal("StartSpan failed, expected non-nil span")
	}
}
