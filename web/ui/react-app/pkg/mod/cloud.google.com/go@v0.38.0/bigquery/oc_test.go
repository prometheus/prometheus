// Copyright 2018 Google LLC
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

package bigquery

import (
	"context"
	"testing"

	"cloud.google.com/go/internal/testutil"
)

func TestOCTracing(t *testing.T) {
	ctx := context.Background()
	client := getClient(t)
	defer client.Close()

	te := testutil.NewTestExporter()
	defer te.Unregister()

	q := client.Query("select *")
	q.Run(ctx) // Doesn't matter if we get an error; span should be created either way

	if len(te.Spans) == 0 {
		t.Fatalf("Expected some spans to be created, but got %d", 0)
	}
}
