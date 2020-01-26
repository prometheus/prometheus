// Copyright 2019, OpenCensus Authors
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

package ocagent

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.opencensus.io/resource"

	resourcepb "github.com/census-instrumentation/opencensus-proto/gen-go/resource/v1"
)

func TestResourceDetector(t *testing.T) {
	ocexp, err := NewExporter(
		WithInsecure(),
		WithAddress(":0"),
		WithResourceDetector(customResourceDetector),
	)
	if err != nil {
		t.Fatalf("Failed to create the ocagent exporter: %v", err)
	}
	defer ocexp.Stop()

	got := ocexp.resource
	want := &resourcepb.Resource{
		Type:   "foo",
		Labels: map[string]string{},
	}
	if !cmp.Equal(got, want) {
		t.Fatalf("Resource detection failed. got %v, want %v\n", got, want)
	}
}

func customResourceDetector(context.Context) (*resource.Resource, error) {
	res := &resource.Resource{
		Type:   "foo",
		Labels: map[string]string{},
	}
	return res, nil
}
