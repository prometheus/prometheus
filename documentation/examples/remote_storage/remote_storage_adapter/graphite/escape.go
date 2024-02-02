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
	"bytes"
	"fmt"
	"strings"

	"github.com/prometheus/common/model"
)

const (
	// From https://github.com/graphite-project/graphite-web/blob/master/webapp/graphite/render/grammar.py#L83
	symbols    = "(){},=.'\"\\"
	printables = ("0123456789abcdefghijklmnopqrstuvwxyz" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"!\"#$%&\\'()*+,-./:;<=>?@[\\]^_`{|}~")
)

// Graphite doesn't support tags, so label names and values must be
// encoded into the metric path. The list of characters that are usable
// with Graphite is rather fuzzy. One 'source of truth' might be the grammar
// used to parse requests in the webapp:
// https://github.com/graphite-project/graphite-web/blob/master/webapp/graphite/render/grammar.py#L83
// The list of valid symbols is defined as:
//   legal = printables - symbols + escaped(symbols)
//
// The default storage backend for Graphite (whisper) stores data
// in filenames, so we also need to use only valid filename characters.
// Fortunately on UNIX only '/' isn't, and Windows is completely unsupported
// by Graphite: http://graphite.readthedocs.org/en/latest/install.html#windows-users

// escape escapes a model.LabelValue into runes allowed in Graphite. The runes
// allowed in Graphite are all single-byte. This function encodes the arbitrary
// byte sequence found in this TagValue in way very similar to the traditional
// percent-encoding (https://en.wikipedia.org/wiki/Percent-encoding):
//
//   - The string that underlies TagValue is scanned byte by byte.
//
//   - If a byte represents a legal Graphite rune with the exception of '%', '/',
//     '=' and '.', that byte is directly copied to the resulting byte slice.
//     % is used for percent-encoding of other bytes.
//     / is not usable in filenames.
//     = is used when generating the path to associate values to labels.
//     . already means something for Graphite and thus can't be used in a value.
//
//   - If the byte is any of (){},=.'"\, then a '\' will be prepended to it. We
//     do not percent-encode them since they are explicitly usable in this
//     way in Graphite.
//
//   - All other bytes are replaced by '%' followed by two bytes containing the
//     uppercase ASCII representation of their hexadecimal value.
//
// This encoding allows to save arbitrary Go strings in Graphite. That's
// required because Prometheus label values can contain anything. Using
// percent encoding makes it easy to unescape, even in javascript.
//
// Examples:
//
//	"foo-bar-42" -> "foo-bar-42"
//
//	"foo_bar%42" -> "foo_bar%2542"
//
//	"http://example.org:8080" -> "http:%2F%2Fexample%2Eorg:8080"
//
//	"Björn's email: bjoern@soundcloud.com" ->
//	"Bj%C3%B6rn's%20email:%20bjoern%40soundcloud.com"
//
//	"日" -> "%E6%97%A5"
func escape(tv model.LabelValue) string {
	length := len(tv)
	result := bytes.NewBuffer(make([]byte, 0, length))
	for i := 0; i < length; i++ {
		b := tv[i]
		switch {
		// . is reserved by graphite, % is used to escape other bytes.
		case b == '.' || b == '%' || b == '/' || b == '=':
			fmt.Fprintf(result, "%%%X", b)
		// These symbols are ok only if backslash escaped.
		case strings.IndexByte(symbols, b) != -1:
			result.WriteString("\\" + string(b))
		// These are all fine.
		case strings.IndexByte(printables, b) != -1:
			result.WriteByte(b)
		// Defaults to percent-encoding.
		default:
			fmt.Fprintf(result, "%%%X", b)
		}
	}
	return result.String()
}
