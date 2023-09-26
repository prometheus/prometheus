// Copyright 2015 The Prometheus Authors
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

package graphite

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

var metric = model.Metric{
	model.MetricNameLabel: "test:metric",
	"testlabel":           "test:value",
	"many_chars":          "abc!ABC:012-3!45ö67~89./(){},=.\"\\",
}

func TestEscape(t *testing.T) {
	// Can we correctly keep and escape valid chars.
	value := "abzABZ019(){},'\"\\"
	expected := "abzABZ019\\(\\)\\{\\}\\,\\'\\\"\\\\"
	actual := escape(model.LabelValue(value))
	require.Equal(t, expected, actual)

	// Test percent-encoding.
	value = "é/|_;:%."
	expected = "%C3%A9%2F|_;:%25%2E"
	actual = escape(model.LabelValue(value))
	require.Equal(t, expected, actual)
}

func TestPathFromMetric(t *testing.T) {
	expected := ("prefix." +
		"test:metric" +
		".many_chars.abc!ABC:012-3!45%C3%B667~89%2E%2F\\(\\)\\{\\}\\,%3D%2E\\\"\\\\" +
		".testlabel.test:value")
	actual := pathFromMetric(metric, "prefix.")
	require.Equal(t, expected, actual)
}
