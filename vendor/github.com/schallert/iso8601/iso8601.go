// The iso8601 package encodes and decodes time.Time in JSON in
// ISO 8601 format, without subsecond resolution or time zone info.
package iso8601

import (
	"net/url"
	"time"
)

const Format = "2006-01-02T15:04:05"
const jsonFormat = `"` + Format + `"`

var fixedZone = time.FixedZone("", 0)

type Time time.Time

// New constructs a new iso8601.Time instance from an existing
// time.Time instance.  This causes the nanosecond field to be set to
// 0, and its time zone set to a fixed zone with no offset from UTC
// (but it is *not* UTC itself).
func New(t time.Time) Time {
	return Time(time.Date(
		t.Year(),
		t.Month(),
		t.Day(),
		t.Hour(),
		t.Minute(),
		t.Second(),
		0,
		fixedZone,
	))
}

func (it Time) MarshalJSON() ([]byte, error) {
	return []byte(time.Time(it).Format(jsonFormat)), nil
}

func (it *Time) UnmarshalJSON(data []byte) error {
	t, err := time.ParseInLocation(jsonFormat, string(data), fixedZone)
	if err == nil {
		*it = Time(t)
	}

	return err
}

func (it Time) String() string {
	return time.Time(it).String()
}

// ISOString returns a string version of the time in its ISO8601 format
func (it Time) ISOString() string {
	return time.Time(it).Format(Format)
}

// Implements the google/go-querystring `Encoder` interface so that
// our ISO 8601 time can be encoded in both JSON and query string format
func (it Time) EncodeValues(key string, v *url.Values) error {
	v.Add(key, time.Time(it).Format(Format))
	return nil
}
