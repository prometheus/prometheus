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

package prometheus

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeLabel(t *testing.T) {
	tests := []struct {
		label        string
		expected     string
		expectedUTF8 string
	}{
		{"", "", ""},
		{"label:with:colons", "label_with_colons", "label:with:colons"}, // Without UTF-8 support, colons are only allowed in metric names
		{"LabelWithCapitalLetters", "LabelWithCapitalLetters", "LabelWithCapitalLetters"},
		{"label!with&special$chars)", "label_with_special_chars_", "label!with&special$chars)"},
		{"label_with_foreign_characters_字符", "label_with_foreign_characters___", "label_with_foreign_characters_字符"},
		{"label.with.dots", "label_with_dots", "label.with.dots"},
		{"123label", "key_123label", "123label"},
		{"_label_starting_with_underscore", "key_label_starting_with_underscore", "_label_starting_with_underscore"},
		{"__label_starting_with_2underscores", "__label_starting_with_2underscores", "__label_starting_with_2underscores"},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("test_%d", i), func(t *testing.T) {
			result := NormalizeLabel(test.label, false)
			require.Equal(t, test.expected, result)
			uTF8result := NormalizeLabel(test.label, true)
			require.Equal(t, test.expectedUTF8, uTF8result)
		})
	}
}
