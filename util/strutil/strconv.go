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
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/grafana/regexp"
)

var invalidLabelCharRE = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// TableLinkForExpression creates an escaped relative link to the table view of
// the provided expression.
func TableLinkForExpression(expr string) string {
	escapedExpression := url.QueryEscape(expr)
	return fmt.Sprintf("/graph?g0.expr=%s&g0.tab=1", escapedExpression)
}

// GraphLinkForExpression creates an escaped relative link to the graph view of
// the provided expression.
func GraphLinkForExpression(expr string) string {
	escapedExpression := url.QueryEscape(expr)
	return fmt.Sprintf("/graph?g0.expr=%s&g0.tab=0", escapedExpression)
}

// SanitizeLabelName replaces anything that doesn't match
// client_label.LabelNameRE with an underscore.
// Note: this does not handle all Prometheus label name restrictions (such as
// not starting with a digit 0-9), and hence should only be used if the label
// name is prefixed with a known valid string.
func SanitizeLabelName(name string) string {
	return invalidLabelCharRE.ReplaceAllString(name, "_")
}

// Helper for SanitizeFullLabelName: validLabelChar[b] is true for bytes that are valid anywhere in a label name.
var validLabelChar = func() (t [256]bool) {
	for c := 'a'; c <= 'z'; c++ {
		t[c] = true
		t[c-'a'+'A'] = true
	}
	for c := '0'; c <= '9'; c++ {
		t[c] = true
	}
	t['_'] = true
	return t
}()

// SanitizeFullLabelName replaces any invalid character with an underscore, and
// if given an empty string, returns a string containing a single underscore.
func SanitizeFullLabelName(name string) string {
	if name == "" {
		return "_"
	}
	// Fast path: find the first byte that needs replacing, without allocating.
	i := 0
	if validLabelChar[name[0]] && name[0] > '9' {
		for i = 1; i < len(name) && validLabelChar[name[i]]; i++ {
		}
		if i == len(name) {
			return name
		}
	}
	var sb strings.Builder
	sb.Grow(len(name))
	sb.WriteString(name[:i])
	// Replace first invalid character with an underscore.
	_, w := utf8.DecodeRuneInString(name[i:])
	i += w
	sb.WriteByte('_')
	for i < len(name) {
		c := name[i]
		switch {
		case c >= utf8.RuneSelf:
			// One underscore per (possibly invalid) rune.
			_, w := utf8.DecodeRuneInString(name[i:])
			i += w
			sb.WriteByte('_')
		case validLabelChar[c]:
			sb.WriteByte(c)
			i++
		default:
			sb.WriteByte('_')
			i++
		}
	}
	return sb.String()
}
