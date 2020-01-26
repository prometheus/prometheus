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

// +build bigtest

// An integration test for PDML using a relatively large database.

package spanner

import (
	"context"
	"fmt"
	"testing"
)

func TestIntegration_BigPDML(t *testing.T) {
	const nRows int = 1e4

	ctx := context.Background()
	client, _, cleanup := prepareIntegrationTest(ctx, t, singerDBStatements)
	defer cleanup()

	columns := []string{"SingerId", "FirstName", "LastName"}

	// Populate the Singers table with random data.
	const rowsPerApply = 1000
	for i := 0; i < nRows; i += rowsPerApply {
		var muts []*Mutation
		for j := 0; j < rowsPerApply; j++ {
			id := i + j
			row := []interface{}{id, fmt.Sprintf("FirstName%d", id), fmt.Sprintf("LastName%d", id)}
			muts = append(muts, Insert("Singers", columns, row))
		}
		if _, err := client.Apply(ctx, muts); err != nil {
			t.Fatal(err)
		}
	}

	// Run a PDML statement.
	count, err := client.PartitionedUpdate(ctx, Statement{
		SQL: `UPDATE Singers SET Singers.FirstName = "changed" WHERE Singers.SingerId != -1`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(nRows); count != want {
		t.Errorf("got %d, want %d", count, want)
	}
}
