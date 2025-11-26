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
package prometheusremotewrite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEveryNTimes(t *testing.T) {
	const n = 128
	ctx, cancel := context.WithCancel(context.Background())
	e := &everyNTimes{
		n: n,
	}

	for range n {
		require.NoError(t, e.checkContext(ctx))
	}

	cancel()
	for range n - 1 {
		require.NoError(t, e.checkContext(ctx))
	}
	require.EqualError(t, e.checkContext(ctx), context.Canceled.Error())
	// e should remember the error.
	require.EqualError(t, e.checkContext(ctx), context.Canceled.Error())
}
