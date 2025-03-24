// Copyright 2013 The Prometheus Authors
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

	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestLabelsToOpenAPILabelSet(t *testing.T) {
	require.Equal(t, models.LabelSet{"aaa": "111", "bbb": "222"}, labelsToOpenAPILabelSet(labels.FromStrings("aaa", "111", "bbb", "222")))
}
