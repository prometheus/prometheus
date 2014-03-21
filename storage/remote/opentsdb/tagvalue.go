package opentsdb

import (
	"fmt"

	clientmodel "github.com/prometheus/client_golang/model"
)

// TagValue is a clientmodel.LabelValue that implements json.Marshaler and
// json.Unmarshaler. These implementations avoid characters illegal in
// OpenTSDB. See the MarshalJSON for details. TagValue is used for the values of
// OpenTSDB tags as well as for OpenTSDB metric names.
type TagValue clientmodel.LabelValue

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
// "foo-bar-42" -> "foo-bar-42"
//
// "foo_bar_42" -> "foo__bar__42"
//
// "http://example.org:8080" -> "http_.//example.org_.8080"
//
// "Björn's email: bjoern@soundcloud.com" ->
// "Bj_C3_B6rn_27s_20email_._20bjoern_40soundcloud.com"
//
// "日" -> "_E6_97_A5"
func (tv TagValue) MarshalJSON() ([]byte, error) {
	length := len(tv)
	result := make([]byte, 1, length+2) // Need at least that many bytes.
	result[0] = '"'
	for i := 0; i < length; i++ {
		b := tv[i]
		switch {
		case (b > 44 && b < 58) || // '-': 45, '.': 46, '/': 47, 0-9: 48-57
			(b > 64 && b < 91) || // A-Z: 65-90
			(b > 96 && b < 123): // a-z: 97-122,
			result = append(result, b)
		case b == 95: // '_'
			result = append(result, 95, 95)
		case b == 58: // ':'
			result = append(result, 95, 46)
		default:
			result = append(result, fmt.Sprintf("_%X", b)...)
		}
	}
	result = append(result, '"')
	return result, nil
}

// UnmarshalJSON unmarshals JSON strings coming from OpenTSDB into Go strings
// by applying the inverse of what is described for the MarshalJSON method.
func (tv *TagValue) UnmarshalJSON(json []byte) error {
	escapeLevel := 0 // How many bytes after '_'.
	var parsedByte byte

	// Might need fewer bytes, but let's avoid realloc.
	result := make([]byte, 0, len(json)-2)

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
			if b == 95 {
				escapeLevel = 1
				continue
			}
			result = append(result, b)
		case 1:
			switch {
			case b == 95:
				result = append(result, 95) // '_'
				escapeLevel = 0
			case b == 46:
				result = append(result, 58) // ':'
				escapeLevel = 0
			case b > 47 && b < 58: // 0-9
				parsedByte = (b - 48) << 4
				escapeLevel = 2
			case b > 64 && b < 71: // A-F
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
			case b > 47 && b < 58: // 0-9
				parsedByte += b - 48
			case b > 64 && b < 71: // A-F
				parsedByte += b - 55
			default:
				return fmt.Errorf(
					"illegal escape sequence at byte %d (%c)",
					i, b,
				)
			}
			result = append(result, parsedByte)
			escapeLevel = 0
		default:
			panic("unexpected escape level")
		}
	}
	*tv = TagValue(result)
	return nil
}
