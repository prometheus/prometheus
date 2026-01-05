// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package opentsdb

import (
	"bytes"
	"fmt"

	"github.com/prometheus/common/model"
)

// TagValue is a model.LabelValue that implements json.Marshaler and
// json.Unmarshaler. These implementations avoid characters illegal in
// OpenTSDB. See the MarshalJSON for details. TagValue is used for the values of
// OpenTSDB tags as well as for OpenTSDB metric names.
type TagValue model.LabelValue

// MarshalJSON marshals this TagValue into JSON that only contains runes allowed
// in OpenTSDB. It implements json.Marshaler. The runes allowed in OpenTSDB are
// all single-byte. This function encodes the arbitrary byte sequence found in
// this TagValue in the following way:
//
// - The string that underlies TagValue is scanned byte by byte.
//
// - If a byte represents a legal OpenTSDB rune with the exception of '_', that
// byte is directly copied to the resulting JSON byte slice.
//
// - If '_' is encountered, it is replaced by '__'.
//
// - If ':' is encountered, it is replaced by '_.'.
//
// - All other bytes are replaced by '_' followed by two bytes containing the
// uppercase ASCII representation of their hexadecimal value.
//
// This encoding allows to save arbitrary Go strings in OpenTSDB. That's
// required because Prometheus label values can contain anything, and even
// Prometheus metric names may (and often do) contain ':' (which is disallowed
// in OpenTSDB strings). The encoding uses '_' as an escape character and
// renders a ':' more or less recognizable as '_.'
//
// Examples:
//
//	"foo-bar-42" -> "foo-bar-42"
//
//	"foo_bar_42" -> "foo__bar__42"
//
//	"http://example.org:8080" -> "http_.//example.org_.8080"
//
//	"Björn's email: bjoern@soundcloud.com" ->
//	"Bj_C3_B6rn_27s_20email_._20bjoern_40soundcloud.com"
//
//	"日" -> "_E6_97_A5"
func (tv TagValue) MarshalJSON() ([]byte, error) {
	length := len(tv)
	// Need at least two more bytes than in tv.
	result := bytes.NewBuffer(make([]byte, 0, length+2))
	result.WriteByte('"')
	for i := range length {
		b := tv[i]
		switch {
		case (b >= '-' && b <= '9') || // '-', '.', '/', 0-9
			(b >= 'A' && b <= 'Z') ||
			(b >= 'a' && b <= 'z'):
			result.WriteByte(b)
		case b == '_':
			result.WriteString("__")
		case b == ':':
			result.WriteString("_.")
		default:
			fmt.Fprintf(result, "_%X", b)
		}
	}
	result.WriteByte('"')
	return result.Bytes(), nil
}

// UnmarshalJSON unmarshals JSON strings coming from OpenTSDB into Go strings
// by applying the inverse of what is described for the MarshalJSON method.
func (tv *TagValue) UnmarshalJSON(json []byte) error {
	escapeLevel := 0 // How many bytes after '_'.
	var parsedByte byte

	// Might need fewer bytes, but let's avoid realloc.
	result := bytes.NewBuffer(make([]byte, 0, len(json)-2))

	for i, b := range json {
		if i == 0 {
			if b != '"' {
				return fmt.Errorf("expected '\"', got %q", b)
			}
			continue
		}
		if i == len(json)-1 {
			if b != '"' {
				return fmt.Errorf("expected '\"', got %q", b)
			}
			break
		}
		switch escapeLevel {
		case 0:
			if b == '_' {
				escapeLevel = 1
				continue
			}
			result.WriteByte(b)
		case 1:
			switch {
			case b == '_':
				result.WriteByte('_')
				escapeLevel = 0
			case b == '.':
				result.WriteByte(':')
				escapeLevel = 0
			case b >= '0' && b <= '9':
				parsedByte = (b - 48) << 4
				escapeLevel = 2
			case b >= 'A' && b <= 'F': // A-F
				parsedByte = (b - 55) << 4
				escapeLevel = 2
			default:
				return fmt.Errorf(
					"illegal escape sequence at byte %d (%c)",
					i, b,
				)
			}
		case 2:
			switch {
			case b >= '0' && b <= '9':
				parsedByte += b - 48
			case b >= 'A' && b <= 'F': // A-F
				parsedByte += b - 55
			default:
				return fmt.Errorf(
					"illegal escape sequence at byte %d (%c)",
					i, b,
				)
			}
			result.WriteByte(parsedByte)
			escapeLevel = 0
		default:
			panic("unexpected escape level")
		}
	}
	*tv = TagValue(result.String())
	return nil
}
