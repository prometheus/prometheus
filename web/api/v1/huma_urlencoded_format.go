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

package v1

import (
	"net/url"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gorilla/schema"
)

// schemaDecoder is a package-level gorilla/schema decoder instance.
// It's shared safely across goroutines and caches struct metadata for performance.
// gorilla/schema automatically uses UnmarshalText for types that implement encoding.TextUnmarshaler.
var schemaDecoder = newSchemaDecoder()

func newSchemaDecoder() *schema.Decoder {
	d := schema.NewDecoder()
	d.SetAliasTag("schema")
	return d
}

// urlEncodedFormat is a custom Huma format for parsing application/x-www-form-urlencoded data.
// It uses gorilla/schema to decode URL-encoded form data directly into structs.
var urlEncodedFormat = huma.Format{
	Marshal: nil,
	Unmarshal: func(data []byte, v any) error {
		values, err := url.ParseQuery(string(data))
		if err != nil {
			return err
		}

		// During validation, Huma first parses the body into map[string]any
		// before parsing it into the target struct.
		if vPtr, ok := v.(*interface{}); ok {
			m := map[string]any{}
			for k, v := range values {
				if len(v) > 1 {
					m[k] = v
				} else if len(v) == 1 {
					m[k] = v[0]
				}
			}
			*vPtr = m
			return nil
		}

		// For struct unmarshaling, use gorilla/schema to decode directly.
		err = schemaDecoder.Decode(v, values)
		return err
	},
}
