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

package spanner

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	stestutil "cloud.google.com/go/spanner/internal/testutil"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

// Check that stats are being exported.
func TestOCStats(t *testing.T) {
	te := testutil.NewTestExporter()
	defer te.Unregister()

	ms := stestutil.NewMockCloudSpanner(t, trxTs)
	ms.Serve()
	ctx := context.Background()
	c, err := NewClient(ctx, "projects/P/instances/I/databases/D",
		option.WithEndpoint(ms.Addr()),
		option.WithGRPCDialOption(grpc.WithInsecure()),
		option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.Single().ReadRow(ctx, "Users", Key{"alice"}, []string{"email"})
	// Wait until we see data from the view.
	select {
	case <-te.Stats:
	case <-time.After(1 * time.Second):
		t.Fatal("no stats were exported before timeout")
	}
}
