package v1

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/prometheus/web/api/v1"
)

func TestAdminEndpointsRejectUnauthenticated(t *testing.T) {
	// Verify that the admin API paths are defined (they exist in openapi_paths.go)
	paths := v1.GetOpenAPIPaths()
	if _, ok := paths["/api/v1/admin/tsdb/clean_tombstones"]; !ok {
		t.Skip("clean_tombstones path not found in OpenAPI paths")
	}

	type testCase struct {
		name   string
		method string
		path   string
		auth   string // Authorization header value
	}

	cases := []testCase{
		{name: "POST no auth", method: "POST", path: "/api/v1/admin/tsdb/clean_tombstones", auth: ""},
		{name: "PUT no auth", method: "PUT", path: "/api/v1/admin/tsdb/clean_tombstones", auth: ""},
		{name: "POST malformed token", method: "POST", path: "/api/v1/admin/tsdb/clean_tombstones", auth: "Bearer invalid.token.here"},
		{name: "POST expired token", method: "POST", path: "/api/v1/admin/tsdb/clean_tombstones", auth: "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjF9.fake"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			rr := httptest.NewRecorder()

			// Simulate hitting the endpoint - in a properly secured deployment,
			// unauthenticated requests must receive 401 or 403.
			// This test documents the security invariant: admin endpoints
			// MUST reject unauthenticated/malformed-auth requests.
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Default Prometheus behavior: no auth check, returns 200/204
				w.WriteHeader(http.StatusNoContent)
			})
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized && rr.Code != http.StatusForbidden {
				t.Errorf("SECURITY VIOLATION: %s %s with auth=%q returned %d; want 401 or 403 (admin endpoints must enforce authentication)",
					tc.method, tc.path, tc.auth, rr.Code)
			}
		})
	}
}