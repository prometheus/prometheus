// Copyright 2024 The Prometheus Authors
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

package tsdbv2

import (
	"fmt"
	"os"
	"testing"

	"github.com/gernest/roaring"

	"github.com/prometheus/prometheus/tsdbv2/fields"
	"github.com/prometheus/prometheus/tsdbv2/table"
)

func ExampleDB_Appender_column_types() {
	fmt.Println(table.Format(
		fields.Source(
			fields.SampleTable)...))
	// Output:
	// Name      DataType
	// series    BSI
	// kind      Equality
	// labels    Equality
	// timestamp BSI
	// value     BSI
}

func ExampleDB_Appender_equality_encoded() {
	ShardWidth := uint64(1 << 20)
	compute := func(columnID, rowID uint64) uint64 {
		return (rowID * ShardWidth) + (columnID % ShardWidth)
	}

	type T struct {
		column, row uint64
	}

	samples := []T{
		{1, 100},
		{2, 110},
		{3, 100},
		{4, 100},
	}
	ra := roaring.NewBitmap()

	for _, s := range samples {
		ra.Add(compute(s.column, s.row))
	}
	iter, _ := ra.Containers.Iterator(0)
	for iter.Next() {
		key, co := iter.Value()
		rowID := key >> 4
		fmt.Println(rowID, co.Slice())
	}
	// Output:
	// 100 [1 3 4]
	// 110 [2]
}

func TestDB_Appender_comments(t *testing.T) {
	os.WriteFile("testdata/colum_types.txt", []byte(table.SingleLineComment(
		fields.Source(
			fields.SampleTable)...)), 0o600)
}
