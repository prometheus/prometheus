// Copyright 2013 The Prometheus Authors
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

package httputil

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/prometheus/prometheus/promql"
)

// GetQueryParams calls r.ParseForm and returns r.Form.
func GetQueryParams(r *http.Request) url.Values {
	r.ParseForm()
	return r.Form
}

var jsonFormatVersion = 1

// ErrorJSON writes the given error JSON-formatted to w.
func ErrorJSON(w io.Writer, err error) error {
	data := struct {
		Type    string `json:"type"`
		Value   string `json:"value"`
		Version int    `json:"version"`
	}{
		Type:    "error",
		Value:   err.Error(),
		Version: jsonFormatVersion,
	}
	enc := json.NewEncoder(w)
	return enc.Encode(data)
}

// RespondJSON converts the given data value to JSON and writes it to w.
func RespondJSON(w io.Writer, val promql.Value) error {
	data := struct {
		Type    string      `json:"type"`
		Value   interface{} `json:"value"`
		Version int         `json:"version"`
	}{
		Type:    val.Type().String(),
		Value:   val,
		Version: jsonFormatVersion,
	}
	// TODO(fabxc): Adding MarshalJSON to promql.Values might be a good idea.
	if sc, ok := val.(*promql.Scalar); ok {
		data.Value = sc.Value
	}
	enc := json.NewEncoder(w)
	return enc.Encode(data)
}
