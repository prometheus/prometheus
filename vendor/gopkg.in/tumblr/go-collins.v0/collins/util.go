package collins

import (
	"net/url"
	"reflect"

	"github.com/google/go-querystring/query"
)

// Credit to
// https://github.com/google/go-github/blob/7277108aa3e8823e0e028f6c74aea2f4ce4a1b5a/github/github.go#L102-L122
// addOptions adds the parameters in opt as URL query parameters to s.  opt
// must be a struct whose fields may contain "url" tags.
func addOptions(s string, opt interface{}) (string, error) {
	v := reflect.ValueOf(opt)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return s, nil
	}

	u, err := url.Parse(s)
	if err != nil {
		return s, err
	}

	qs, err := query.Values(opt)
	if err != nil {
		return s, err
	}

	u.RawQuery = qs.Encode()
	return u.String(), nil
}
