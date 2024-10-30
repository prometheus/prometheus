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
		label    string
		expected string
	}{
		{"", ""},
		{"label:with:colons", "label:with:colons"},
		{"label!with&special$chars)", "label_with_special_chars_"},
		{"label_with_foreign_characteres_字符", "label_with_foreign_characteres___"},
		{"label.with.dots", "label_with_dots"},
		{"123label", "key_123label"},
		{"_label", "key_label"},
		{"__label", "__label"},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("test_%d", i), func(t *testing.T) {
			result := NormalizeLabel(test.label)
			require.Equal(t, test.expected, result)
		})
	}
}
