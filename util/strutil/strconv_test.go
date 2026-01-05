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

package strutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type linkTest struct {
	expression        string
	expectedGraphLink string
	expectedTableLink string
}

var linkTests = []linkTest{
	{
		"sum(incoming_http_requests_total) by (system)",
		"/graph?g0.expr=sum%28incoming_http_requests_total%29+by+%28system%29&g0.tab=0",
		"/graph?g0.expr=sum%28incoming_http_requests_total%29+by+%28system%29&g0.tab=1",
	},
	{
		"sum(incoming_http_requests_total{system=\"trackmetadata\"})",
		"/graph?g0.expr=sum%28incoming_http_requests_total%7Bsystem%3D%22trackmetadata%22%7D%29&g0.tab=0",
		"/graph?g0.expr=sum%28incoming_http_requests_total%7Bsystem%3D%22trackmetadata%22%7D%29&g0.tab=1",
	},
}

func TestLink(t *testing.T) {
	for _, tt := range linkTests {
		graphLink := GraphLinkForExpression(tt.expression)
		require.Equal(t, tt.expectedGraphLink, graphLink,
			"GraphLinkForExpression failed for expression (%#q)", tt.expression)

		tableLink := TableLinkForExpression(tt.expression)
		require.Equal(t, tt.expectedTableLink, tableLink,
			"TableLinkForExpression failed for expression (%#q)", tt.expression)
	}
}

func TestSanitizeLabelName(t *testing.T) {
	actual := SanitizeLabelName("fooClientLABEL")
	expected := "fooClientLABEL"
	require.Equal(t, expected, actual, "SanitizeLabelName failed for label (%s)", expected)

	actual = SanitizeLabelName("barClient.LABEL$$##")
	expected = "barClient_LABEL____"
	require.Equal(t, expected, actual, "SanitizeLabelName failed for label (%s)", expected)
}

func TestSanitizeFullLabelName(t *testing.T) {
	actual := SanitizeFullLabelName("fooClientLABEL")
	expected := "fooClientLABEL"
	require.Equal(t, expected, actual, "SanitizeFullLabelName failed for label (%s)", expected)

	actual = SanitizeFullLabelName("barClient.LABEL$$##")
	expected = "barClient_LABEL____"
	require.Equal(t, expected, actual, "SanitizeFullLabelName failed for label (%s)", expected)

	actual = SanitizeFullLabelName("0zerothClient1LABEL")
	expected = "_zerothClient1LABEL"
	require.Equal(t, expected, actual, "SanitizeFullLabelName failed for label (%s)", expected)

	actual = SanitizeFullLabelName("")
	expected = "_"
	require.Equal(t, expected, actual, "SanitizeFullLabelName failed for the empty label")
}
