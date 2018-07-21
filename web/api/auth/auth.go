// Copyright 2018 The Prometheus Authors
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

package auth

import (
	"context"
	"net/http"
	"strings"
)

type ctxKey string

var (
	ctxKeyBearerToken = ctxKey("bearer-token")
)

func WithBearerToken(ctx context.Context, r *http.Request) context.Context {
	authorization := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
		return context.WithValue(ctx, ctxKeyBearerToken, authorization[7:])
	} else {
		return ctx
	}

}

func GetBearerToken(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(ctxKeyBearerToken).(string)
	return token, ok
}
