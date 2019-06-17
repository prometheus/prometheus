// +build go1.7

package httprouter

import (
	"context"
	"net/http"
)

type paramsKey struct{}

// ParamsKey is the request context key under which URL params are stored.
//
// This is only present from go 1.7.
var ParamsKey = paramsKey{}

// Handler is an adapter which allows the usage of an http.Handler as a
// request handle. With go 1.7+, the Params will be available in the
// request context under ParamsKey.
func (r *Router) Handler(method, path string, handler http.Handler) {
	r.Handle(method, path,
		func(w http.ResponseWriter, req *http.Request, p Params) {
			ctx := req.Context()
			ctx = context.WithValue(ctx, ParamsKey, p)
			req = req.WithContext(ctx)
			handler.ServeHTTP(w, req)
		},
	)
}

// ParamsFromContext pulls the URL parameters from a request context,
// or returns nil if none are present.
//
// This is only present from go 1.7.
func ParamsFromContext(ctx context.Context) Params {
	p, _ := ctx.Value(ParamsKey).(Params)
	return p
}
