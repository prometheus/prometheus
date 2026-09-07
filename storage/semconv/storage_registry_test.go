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
	"strconv"
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

func registryWithSemconv(contents []byte) map[string][]byte {
	return map[string][]byte{
		"registry.yaml": []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.0.0
versions:
  1.0.0:
`),
		"1.0.0": contents,
	}
}

func writeAttributeRefs(b *strings.Builder, prefix string, count int) {
	for i := range count {
		b.WriteString("      - ref: ")
		b.WriteString(prefix)
		b.WriteString(strconv.Itoa(i))
		b.WriteByte('\n')
	}
}

func inheritedAttributeLimitRegistry(parentAttrs, metricAttrs int) map[string][]byte {
	var b strings.Builder
	b.WriteString(`groups:
  - id: attributes.base
    type: attribute_group
    attributes:
`)
	writeAttributeRefs(&b, "base.", parentAttrs)
	b.WriteString(`  - id: metric.queue.depth
    type: metric
    metric_name: queue.depth
    extends: attributes.base
    attributes:
`)
	writeAttributeRefs(&b, "metric.", metricAttrs)
	return registryWithSemconv([]byte(b.String()))
}

func inheritedAttributeFanoutRegistry(children int) map[string][]byte {
	var b strings.Builder
	b.WriteString(`groups:
  - id: attributes.base
    type: attribute_group
    attributes:
`)
	writeAttributeRefs(&b, "base.", 256)
	for i := range children {
		b.WriteString("  - id: attributes.child.")
		b.WriteString(strconv.Itoa(i))
		b.WriteString("\n    type: attribute_group\n    extends: attributes.base\n")
	}
	return registryWithSemconv([]byte(b.String()))
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

	t.Run("rejects invalid unused semconv metadata", func(t *testing.T) {
		_, err := semconv.AwareStorageWithRegistry(teststorage.New(t), registryWithSemconv([]byte(`
groups:
  - id: attributes.unused
    type: attribute_group
    extends: attributes.missing
`)))
		require.ErrorContains(t, err, `semconv group "attributes.unused" extends unknown group "attributes.missing"`)
	})

	for _, tc := range []struct {
		name            string
		metricAttrs     int
		wantErrContains string
	}{
		{name: "accepts 256 resolved attributes", metricAttrs: 6},
		{name: "rejects 257 resolved attributes", metricAttrs: 7, wantErrContains: "semconv group attributes would exceed 256"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := semconv.AwareStorageWithRegistry(teststorage.New(t), inheritedAttributeLimitRegistry(250, tc.metricAttrs))
			if tc.wantErrContains != "" {
				require.ErrorContains(t, err, tc.wantErrContains)
				return
			}
			require.NoError(t, err)
		})
	}

	t.Run("rejects excessive cumulative inherited attributes", func(t *testing.T) {
		files := inheritedAttributeFanoutRegistry(256)
		require.Less(t, len(files["1.0.0"]), 16<<20)
		_, err := semconv.AwareStorageWithRegistry(teststorage.New(t), files)
		require.ErrorContains(t, err, "semconv file attribute slots would exceed 65536")
	})

	// An operator registry that mirrors the embedded one must drive the same
	// schema-version rename fan-out, proving the injected source behaves
	// identically to the embedded default.
	t.Run("an operator registry mirroring the embedded one resolves identically", func(t *testing.T) {
		files := readRegistryDir(t, "registry")
		underlying := teststorage.New(t)
		wrapped, err := semconv.AwareStorageWithRegistry(underlying, files)
		require.NoError(t, err)
		// The wrapper must retain its validated copy, not aliases owned by the caller.
		for name, b := range files {
			clear(b)
			files[name] = []byte("mutated after construction")
		}

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
