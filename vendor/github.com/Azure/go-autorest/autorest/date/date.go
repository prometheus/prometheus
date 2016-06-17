/*
Package date provides time.Time derivatives that conform to the Swagger.io (https://swagger.io/)
defined date   formats: Date and DateTime. Both types may, in most cases, be used in lieu of
time.Time types. And both convert to time.Time through a ToTime method.
*/
package date

import (
	"fmt"
	"github.com/Azure/go-autorest/autorest"
	"io/ioutil"
	"net/http"
	"time"
)

const (
	rfc3339FullDate     = "2006-01-02"
	rfc3339FullDateJSON = `"2006-01-02"`
	rfc3339             = "2006-01-02T15:04:05Z07:00"
	rfc3339JSON         = `"2006-01-02T15:04:05Z07:00"`
	dateFormat          = "%4d-%02d-%02d"
	jsonFormat          = `"%4d-%02d-%02d"`
)

// Date defines a type similar to time.Time but assumes a layout of RFC3339 full-date (i.e.,
// 2006-01-02).
type Date struct {
	time.Time
}

// ParseDate create a new Date from the passed string.
func ParseDate(date string) (d Date, err error) {
	return parseDate(date, rfc3339FullDate)
}

func parseDate(date string, format string) (Date, error) {
	d, err := time.Parse(format, date)
	return Date{Time: d}, err
}

// ByUnmarshallingDate returns a RespondDecorator that decodes the http.Response Body into a Date
// pointed to by d.
func ByUnmarshallingDate(d *Date) autorest.RespondDecorator {
	return byUnmarshallingDate(d, rfc3339FullDate)
}

// ByUnmarshallingJSONDate returns a RespondDecorator that decodes JSON within the http.Response
// Body into a Date pointed to by d.
func ByUnmarshallingJSONDate(d *Date) autorest.RespondDecorator {
	return byUnmarshallingDate(d, rfc3339FullDateJSON)
}

func byUnmarshallingDate(d *Date, format string) autorest.RespondDecorator {
	return func(r autorest.Responder) autorest.Responder {
		return autorest.ResponderFunc(func(resp *http.Response) error {
			err := r.Respond(resp)
			if err == nil {
				var b []byte
				b, err = ioutil.ReadAll(resp.Body)
				if err == nil {
					*d, err = parseDate(string(b), format)
				}
			}
			return err
		})
	}
}

// MarshalBinary preserves the Date as a byte array conforming to RFC3339 full-date (i.e.,
// 2006-01-02).
func (d Date) MarshalBinary() ([]byte, error) {
	return d.MarshalText()
}

// UnmarshalBinary reconstitutes a Date saved as a byte array conforming to RFC3339 full-date (i.e.,
// 2006-01-02).
func (d *Date) UnmarshalBinary(data []byte) error {
	return d.UnmarshalText(data)
}

// MarshalJSON preserves the Date as a JSON string conforming to RFC3339 full-date (i.e.,
// 2006-01-02).
func (d Date) MarshalJSON() (json []byte, err error) {
	return []byte(fmt.Sprintf(jsonFormat, d.Year(), d.Month(), d.Day())), nil
}

// UnmarshalJSON reconstitutes the Date from a JSON string conforming to RFC3339 full-date (i.e.,
// 2006-01-02).
func (d *Date) UnmarshalJSON(data []byte) (err error) {
	d.Time, err = time.Parse(rfc3339FullDateJSON, string(data))
	if err != nil {
		return err
	}
	return nil
}

// MarshalText preserves the Date as a byte array conforming to RFC3339 full-date (i.e.,
// 2006-01-02).
func (d Date) MarshalText() (text []byte, err error) {
	return []byte(fmt.Sprintf(dateFormat, d.Year(), d.Month(), d.Day())), nil
}

// UnmarshalText reconstitutes a Date saved as a byte array conforming to RFC3339 full-date (i.e.,
// 2006-01-02).
func (d *Date) UnmarshalText(data []byte) (err error) {
	d.Time, err = time.Parse(rfc3339FullDate, string(data))
	if err != nil {
		return err
	}
	return nil
}

// String returns the Date formatted as an RFC3339 full-date string (i.e., 2006-01-02).
func (d Date) String() string {
	return fmt.Sprintf(dateFormat, d.Year(), d.Month(), d.Day())
}

// ToTime returns a Date as a time.Time
func (d Date) ToTime() time.Time {
	return d.Time
}
