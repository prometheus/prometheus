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
		label     string
		allowUTF8 bool
		expected  string
	}{
		{"", false, ""},
		{"", true, ""},
		{"label:with:colons", false, "label:with:colons"},
		{"label:with:colons", true, "label:with:colons"},
		{"label!with$special&chars)", false, "label_with_special_chars_"},
		{"label!with$special&chars)", true, "label!with$special&chars)"},
		{"label_with_foreign_characteres_字符", false, "label_with_foreign_characteres___"},
		{"label_with_foreign_characteres_字符", true, "label_with_foreign_characteres_字符"},
		{"label.with.dots", false, "label_with_dots"},
		{"label.with.dots", true, "label.with.dots"},
		{"123label", false, "key_123label"},
		{"123label", true, "123label"}, // UTF-8 allows numbers at the beginning
		{"_label", false, "key_label"},
		{"_label", true, "_label"}, // UTF-8 allows single underscores at the beginning
		{"__label", false, "__label"},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("test_%d", i), func(t *testing.T) {
			result := NormalizeLabel(test.label, test.allowUTF8)
			require.Equal(t, test.expected, result)
		})
	}
}
