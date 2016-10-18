package route

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
)

var (
	mtx   = sync.RWMutex{}
	ctxts = map[*http.Request]context.Context{}
)

// Context returns the context for the request.
func Context(r *http.Request) context.Context {
	mtx.RLock()
	defer mtx.RUnlock()
	return ctxts[r]
}

type param string

// Param returns param p for the context.
func Param(ctx context.Context, p string) string {
	return ctx.Value(param(p)).(string)
}

// WithParam returns a new context with param p set to v.
func WithParam(ctx context.Context, p, v string) context.Context {
	return context.WithValue(ctx, param(p), v)
}

type contextFn func(r *http.Request) (context.Context, error)

// Router wraps httprouter.Router and adds support for prefixed sub-routers
// and per-request context injections.
type Router struct {
	rtr    *httprouter.Router
	prefix string
	ctxFn  contextFn
}

// New returns a new Router.
func New(ctxFn contextFn) *Router {
	if ctxFn == nil {
		ctxFn = func(r *http.Request) (context.Context, error) {
			return context.Background(), nil
		}
	}
	return &Router{
		rtr:   httprouter.New(),
		ctxFn: ctxFn,
	}
}

// WithPrefix returns a router that prefixes all registered routes with prefix.
func (r *Router) WithPrefix(prefix string) *Router {
	return &Router{rtr: r.rtr, prefix: r.prefix + prefix, ctxFn: r.ctxFn}
}

// handle turns a HandlerFunc into an httprouter.Handle.
func (r *Router) handle(h http.HandlerFunc) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, params httprouter.Params) {
		reqCtx, err := r.ctxFn(req)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error creating request context: %v", err), http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithCancel(reqCtx)
		defer cancel()

		for _, p := range params {
			ctx = context.WithValue(ctx, param(p.Key), p.Value)
		}

		mtx.Lock()
		ctxts[req] = ctx
		mtx.Unlock()

		h(w, req)

		mtx.Lock()
		delete(ctxts, req)
		mtx.Unlock()
	}
}

// Get registers a new GET route.
func (r *Router) Get(path string, h http.HandlerFunc) {
	r.rtr.GET(r.prefix+path, r.handle(h))
}

// Options registers a new OPTIONS route.
func (r *Router) Options(path string, h http.HandlerFunc) {
	r.rtr.OPTIONS(r.prefix+path, r.handle(h))
}

// Del registers a new DELETE route.
func (r *Router) Del(path string, h http.HandlerFunc) {
	r.rtr.DELETE(r.prefix+path, r.handle(h))
}

// Put registers a new PUT route.
func (r *Router) Put(path string, h http.HandlerFunc) {
	r.rtr.PUT(r.prefix+path, r.handle(h))
}

// Post registers a new POST route.
func (r *Router) Post(path string, h http.HandlerFunc) {
	r.rtr.POST(r.prefix+path, r.handle(h))
}

// Redirect takes an absolute path and sends an internal HTTP redirect for it,
// prefixed by the router's path prefix. Note that this method does not include
// functionality for handling relative paths or full URL redirects.
func (r *Router) Redirect(w http.ResponseWriter, req *http.Request, path string, code int) {
	http.Redirect(w, req, r.prefix+path, code)
}

// ServeHTTP implements http.Handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.rtr.ServeHTTP(w, req)
}

// FileServe returns a new http.HandlerFunc that serves files from dir.
// Using routes must provide the *filepath parameter.
func FileServe(dir string) http.HandlerFunc {
	fs := http.FileServer(http.Dir(dir))

	return func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = Param(Context(r), "filepath")
		fs.ServeHTTP(w, r)
	}
}
