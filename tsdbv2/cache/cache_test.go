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
package cache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLabels(t *testing.T) {
	fields := []uint64{1, 2, 3}
	ids := []uint64{4, 5, 6}
	id := uint64(7)
	encooded := New(id, fields, ids, nil).Encode()

	var gotField, gotID []uint64
	srs := Decode(encooded)
	srs.Iter(func(field, id uint64) {
		gotField = append(gotField, field)
		gotID = append(gotID, id)
	})

	require.Equal(t, id, srs.ID())
	require.Equal(t, fields, gotField)
	require.Equal(t, ids, gotID)
}
