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

package series

import (
	"cmp"

	"github.com/gernest/roaring"
)

type Data struct {
	Rows      *roaring.Bitmap
	Kind      []uint64
	ID        []uint64
	Timestamp []uint64
	Value     []uint64
}

func (d *Data) Release() {
	*d = Data{}
}

func (d *Data) Compare(o *Data) int {
	return cmp.Compare(d.firstRow(), o.firstRow())
}

func Compare(a, b *Data) int {
	return a.Compare(b)
}

func (d *Data) firstRow() uint64 {
	lo, _ := d.Rows.Min()
	return lo
}
