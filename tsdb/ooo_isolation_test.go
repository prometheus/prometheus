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

package tsdb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOOOIsolation(t *testing.T) {
	i := newOOOIsolation()

	// Empty state shouldn't have any open reads.
	require.False(t, i.HasOpenReadsAtOrBefore(0))
	require.False(t, i.HasOpenReadsAtOrBefore(1))
	require.False(t, i.HasOpenReadsAtOrBefore(2))
	require.False(t, i.HasOpenReadsAtOrBefore(3))

	// Add a read.
	read1 := i.TrackReadAfter(1)
	require.False(t, i.HasOpenReadsAtOrBefore(0))
	require.False(t, i.HasOpenReadsAtOrBefore(1))
	require.True(t, i.HasOpenReadsAtOrBefore(2))

	// Add another overlapping read.
	read2 := i.TrackReadAfter(0)
	require.False(t, i.HasOpenReadsAtOrBefore(0))
	require.True(t, i.HasOpenReadsAtOrBefore(1))
	require.True(t, i.HasOpenReadsAtOrBefore(2))

	// Close the second read, should now only report open reads for the first read's ref.
	read2.Close()
	require.False(t, i.HasOpenReadsAtOrBefore(0))
	require.False(t, i.HasOpenReadsAtOrBefore(1))
	require.True(t, i.HasOpenReadsAtOrBefore(2))

	// Close the second read again: this should do nothing and ensures we can safely call Close() multiple times.
	read2.Close()
	require.False(t, i.HasOpenReadsAtOrBefore(0))
	require.False(t, i.HasOpenReadsAtOrBefore(1))
	require.True(t, i.HasOpenReadsAtOrBefore(2))

	// Closing the first read should indicate no further open reads.
	read1.Close()
	require.False(t, i.HasOpenReadsAtOrBefore(0))
	require.False(t, i.HasOpenReadsAtOrBefore(1))
	require.False(t, i.HasOpenReadsAtOrBefore(2))
}
