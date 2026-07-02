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

package semconv_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage/semconv"
	"github.com/prometheus/prometheus/util/teststorage"
)

// readRegistryDir loads an on-disk registry directory into a base-name → bytes
// map, mirroring how an operator supplies a registry via configuration.
func readRegistryDir(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	files := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		files[e.Name()] = b
	}
	require.NotEmpty(t, files)
	return files
}

func TestAwareStorageWithRegistry(t *testing.T) {
	t.Run("rejects an invalid registry", func(t *testing.T) {
		_, err := semconv.AwareStorageWithRegistry(teststorage.New(t), map[string][]byte{
			"registry.yaml": []byte("file_format: 9.9.9\n"),
		})
		require.Error(t, err)
	})

	t.Run("rejects an empty registry", func(t *testing.T) {
		_, err := semconv.AwareStorageWithRegistry(teststorage.New(t), nil)
		require.Error(t, err)
	})

	// An operator registry that mirrors the embedded one must drive the same
	// schema-version rename fan-out, proving the injected source behaves
	// identically to the embedded default.
	t.Run("an operator registry mirroring the embedded one resolves identically", func(t *testing.T) {
		files := readRegistryDir(t, "registry")
		underlying := teststorage.New(t)
		wrapped, err := semconv.AwareStorageWithRegistry(underlying, files)
		require.NoError(t, err)

		// Written under the semconv 1.0.0 name; semconv 1.1.0 renamed it to "test".
		appendSeries(t, wrapped, "test.counter", 1, 7.0, "http.response.status_code", "200")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
		)
		got := collectSeries(t, set)
		require.NotEmpty(t, got, "expected the historical name to surface via the operator registry")
		var found bool
		for k := range got {
			if strings.Contains(k, `__name__="test"`) {
				found = true
			}
		}
		require.True(t, found, "expected the renamed metric under its 1.1.0 name in: %v", got)
	})
}
