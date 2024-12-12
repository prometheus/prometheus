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

package table

import (
	"bufio"
	"bytes"
	"strings"
	"text/tabwriter"
)

func SingleLineComment(lines ...[]string) string {
	return formatPrefix([]byte("//  "), lines...)
}

func Example(lines ...[]string) string {
	return "// Output:\n" + formatPrefix([]byte("// "), lines...)
}

func MultiLineComment(lines ...[]string) string {
	return formatPrefix([]byte("  "), lines...)
}

func formatPrefix(prefix []byte, lines ...[]string) string {
	o := Format(lines...)
	var b bytes.Buffer
	s := bufio.NewScanner(strings.NewReader(o))
	start := true
	for s.Scan() {
		if !start {
			_ = b.WriteByte('\n')
		} else {
			start = false
		}
		_, _ = b.Write(prefix)
		_, _ = b.Write(s.Bytes())
	}
	return b.String()
}

func Format(lines ...[]string) string {
	return formatTable(lines...)
}

func formatTable(lines ...[]string) string {
	var b bytes.Buffer
	w := tabwriter.NewWriter(&b, 0, 0, 1, ' ', 0)

	for _, line := range lines {
		_, _ = w.Write([]byte(strings.Join(line, "\t") + "\n"))
	}
	_ = w.Flush()
	return b.String()
}
