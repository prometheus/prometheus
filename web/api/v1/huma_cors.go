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
	"github.com/danielgtaylor/huma/v2"
	"github.com/grafana/regexp"
)

var corsHeaders = map[string]string{
	"Access-Control-Allow-Headers":  "Accept, Authorization, Content-Type, Origin",
	"Access-Control-Allow-Methods":  "GET, POST, OPTIONS",
	"Access-Control-Expose-Headers": "Date",
}

func CORSMiddleware(o *regexp.Regexp) func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		defer func() { next(ctx) }()
		ctx.SetHeader("Vary", "Origin")
		origin := ctx.Header("Origin")
		if origin == "" {
			return
		}

		for k, v := range corsHeaders {
			ctx.SetHeader(k, v)
		}
		if o.String() == "^(?:.*)$" {
			ctx.SetHeader("Access-Control-Allow-Origin", "*")
			return
		}

		if o.MatchString(origin) {
			ctx.SetHeader("Access-Control-Allow-Origin", origin)
		}
	}
}
