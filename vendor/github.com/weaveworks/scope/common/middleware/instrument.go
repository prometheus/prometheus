package middleware

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
)

// Instrument is a Middleware which records timings for every HTTP request
type Instrument struct {
	RouteMatcher interface {
		Match(*http.Request, *mux.RouteMatch) bool
	}
	Duration *prometheus.HistogramVec
}

func isWSHandshakeRequest(req *http.Request) bool {
	return strings.ToLower(req.Header.Get("Upgrade")) == "websocket" &&
		strings.ToLower(req.Header.Get("Connection")) == "upgrade"
}

// Wrap implements middleware.Interface
func (i Instrument) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		begin := time.Now()
		isWS := strconv.FormatBool(isWSHandshakeRequest(r))
		interceptor := &interceptor{ResponseWriter: w, statusCode: http.StatusOK}
		route := i.getRouteName(r)
		next.ServeHTTP(interceptor, r)
		var (
			status = strconv.Itoa(interceptor.statusCode)
			took   = time.Since(begin)
		)
		i.Duration.WithLabelValues(r.Method, route, status, isWS).Observe(took.Seconds())
	})
}

// Return a name identifier for ths request.  There are three options:
//   1. The request matches a gorilla mux route, with a name.  Use that.
//   2. The request matches an unamed gorilla mux router.  Munge the path
//      template such that templates like '/api/{org}/foo' come out as
//      'api_org_foo'.
//   3. The request doesn't match a mux route.  Munge the Path in the same
//      manner as (2).
// We do all this as we do not wish to emit high cardinality labels to
// prometheus.
func (i Instrument) getRouteName(r *http.Request) string {
	var routeMatch mux.RouteMatch
	if i.RouteMatcher != nil && i.RouteMatcher.Match(r, &routeMatch) {
		if name := routeMatch.Route.GetName(); name != "" {
			return name
		}
		if tmpl, err := routeMatch.Route.GetPathTemplate(); err != nil {
			return MakeLabelValue(tmpl)
		}
	}
	return MakeLabelValue(r.URL.Path)
}

var invalidChars = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// MakeLabelValue converts a Gorilla mux path to a string suitable for use in
// a Prometheus label value.
func MakeLabelValue(path string) string {
	// Convert non-alnums to underscores.
	result := invalidChars.ReplaceAllString(path, "_")

	// Trim leading and trailing underscores.
	result = strings.Trim(result, "_")

	// Make it all lowercase
	result = strings.ToLower(result)

	// Special case.
	if result == "" {
		result = "root"
	}
	return result
}
