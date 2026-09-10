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

package notifier

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/semconv"
)

// TestRegistryContract asserts that the metrics this package declares match the
// semantic convention registry. It inspects the descriptors rather than the
// gathered metrics, so metrics that have not produced a sample are covered too.
func TestRegistryContract(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	newAlertMetrics(reg, func() float64 { return 0 })

	declared := make(map[string]prometheus.DescInfo)
	for _, desc := range reg.DescribeAll() {
		info := desc.Info()
		declared[info.FQName] = info
	}

	registry, err := semconv.LoadFile("../semconv/registry.yaml")
	require.NoError(t, err)

	if diff := registry.Diff(declared); diff != "" {
		require.Fail(t, "registry and code disagree (-registry +code):\n"+diff)
	}
}
