// Copyright The Prometheus Authors
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

package index

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

func TestMemPostingsOutOfOrderAddDoesNotChangeExistingIterator(t *testing.T) {
	p := NewMemPostings()
	lset := labels.FromStrings("a", "x")
	p.Add(2, lset)
	p.Add(3, lset)

	before := p.Postings(t.Context(), "a", "x")
	p.Add(1, lset)

	beforeRefs, err := ExpandPostings(before)
	require.NoError(t, err)
	require.Equal(t, []storage.SeriesRef{2, 3}, beforeRefs)

	afterRefs, err := ExpandPostings(p.Postings(t.Context(), "a", "x"))
	require.NoError(t, err)
	require.Equal(t, []storage.SeriesRef{1, 2, 3}, afterRefs)
}
