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

package zookeeper

import (
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// TestNewDiscoveryError can fail if the DNS resolver mistakenly resolves the domain below.
// See https://github.com/prometheus/prometheus/issues/16191 for a precedent.
func TestNewDiscoveryError(t *testing.T) {
	_, err := NewDiscovery(
		[]string{"unreachable.invalid"},
		time.Second, []string{"/"},
		nil,
		func([]byte, string) (model.LabelSet, error) { return nil, nil })
	require.Error(t, err)
}
