// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	_ "embed"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
)

//go:embed api.go
var apiGoSource string

// routeInfo represents a route extracted from the Register function.
type routeInfo struct {
	method string
	path   string
}

// extractRoutesFromRegister parses the api.go source and extracts all routes
// registered in the (*API) Register function using AST.
func extractRoutesFromRegister(t *testing.T, source string) []routeInfo {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "api.go", source, parser.ParseComments)
	require.NoError(t, err, "failed to parse api.go")

	var registerFunc *ast.FuncDecl

	// Find the Register method on *API.
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}

		if fn.Name.Name != "Register" {
			return true
		}

		// Ensure it's a method on *API.
		if fn.Recv == nil || len(fn.Recv.List) != 1 {
			return true
		}

		star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			return true
		}

		ident, ok := star.X.(*ast.Ident)
		if !ok || ident.Name != "API" {
			return true
		}

		registerFunc = fn
		return false // Stop walking once found.
	})

	require.NotNil(t, registerFunc, "Register method not found")

	var routes []routeInfo

	// Extract all r.Get, r.Post, r.Put, r.Delete, r.Options calls.
	ast.Inspect(registerFunc.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		// Check if it's a router method call.
		method := sel.Sel.Name
		if method != "Get" && method != "Post" && method != "Put" && method != "Delete" && method != "Del" && method != "Options" {
			return true
		}

		// Ensure the receiver is 'r'.
		if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "r" {
			return true
		}

		if len(call.Args) == 0 {
			return true
		}

		// Extract the path from the first argument.
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}

		path, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}

		// Normalize Del to DELETE.
		if method == "Del" {
			method = "Delete"
		}

		routes = append(routes, routeInfo{
			method: strings.ToUpper(method),
			path:   path,
		})
		return true
	})

	return routes
}

// normalizePathForOpenAPI converts route paths with colon parameters to OpenAPI format.
// e.g., "/label/:name/values" -> "/label/{name}/values".
func normalizePathForOpenAPI(path string) string {
	// Replace :param with {param}.
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if trimmed, ok := strings.CutPrefix(part, ":"); ok {
			parts[i] = "{" + trimmed + "}"
		}
	}
	return strings.Join(parts, "/")
}

// TestOpenAPICoverage verifies that all routes registered in the Register function
// are documented in the OpenAPI specification.
func TestOpenAPICoverage(t *testing.T) {
	// Extract routes from api.go using AST.
	routes := extractRoutesFromRegister(t, apiGoSource)
	require.NotEmpty(t, routes, "no routes found in Register function")

	// Build OpenAPI spec.
	builder := NewOpenAPIBuilder(OpenAPIOptions{}, promslog.NewNopLogger())
	allPaths := builder.getAllPathDefinitions()

	// Create a map of OpenAPI paths for quick lookup.
	// Key is the normalized path, value is the PathItem.
	openAPIPaths := make(map[string]bool)
	for pair := allPaths.First(); pair != nil; pair = pair.Next() {
		pathItem := pair.Value()
		path := pair.Key()

		// Track which methods are defined for this path.
		if pathItem.Get != nil {
			openAPIPaths[path+":GET"] = true
		}
		if pathItem.Post != nil {
			openAPIPaths[path+":POST"] = true
		}
		if pathItem.Put != nil {
			openAPIPaths[path+":PUT"] = true
		}
		if pathItem.Delete != nil {
			openAPIPaths[path+":DELETE"] = true
		}
		if pathItem.Options != nil {
			openAPIPaths[path+":OPTIONS"] = true
		}
	}

	// Check coverage for each route.
	var missingRoutes []string
	ignoredRoutes := map[string]bool{
		"/*path:OPTIONS":          true, // Wildcard OPTIONS handler.
		"/openapi.yaml:GET":       true, // Self-referential endpoint.
		"/notifications/live:GET": true, // SSE endpoint (version-specific).
	}

	for _, route := range routes {
		normalizedPath := normalizePathForOpenAPI(route.path)
		key := normalizedPath + ":" + route.method

		// Skip ignored routes.
		if ignoredRoutes[key] {
			continue
		}

		if !openAPIPaths[key] {
			missingRoutes = append(missingRoutes, key)
		}
	}

	if len(missingRoutes) > 0 {
		t.Errorf("The following routes are registered but not documented in OpenAPI spec:\n%s",
			strings.Join(missingRoutes, "\n"))
	}
}

// TestOpenAPIHasNoExtraRoutes verifies that the OpenAPI spec doesn't document
// routes that aren't actually registered.
func TestOpenAPIHasNoExtraRoutes(t *testing.T) {
	// Extract routes from api.go using AST.
	routes := extractRoutesFromRegister(t, apiGoSource)
	require.NotEmpty(t, routes, "no routes found in Register function")

	// Create a map of registered routes.
	registeredRoutes := make(map[string]bool)
	for _, route := range routes {
		normalizedPath := normalizePathForOpenAPI(route.path)
		key := normalizedPath + ":" + route.method
		registeredRoutes[key] = true
	}

	// Build OpenAPI spec.
	builder := NewOpenAPIBuilder(OpenAPIOptions{}, promslog.NewNopLogger())
	allPaths := builder.getAllPathDefinitions()

	// Check if any OpenAPI paths are not registered.
	var extraRoutes []string

	for pair := allPaths.First(); pair != nil; pair = pair.Next() {
		pathItem := pair.Value()
		path := pair.Key()

		checkMethod := func(method string, op *v3.Operation) {
			if op != nil {
				key := path + ":" + method
				if !registeredRoutes[key] {
					extraRoutes = append(extraRoutes, key)
				}
			}
		}

		checkMethod("GET", pathItem.Get)
		checkMethod("POST", pathItem.Post)
		checkMethod("PUT", pathItem.Put)
		checkMethod("DELETE", pathItem.Delete)
		checkMethod("OPTIONS", pathItem.Options)
	}

	if len(extraRoutes) > 0 {
		t.Errorf("The following routes are documented in OpenAPI but not registered:\n%s",
			strings.Join(extraRoutes, "\n"))
	}
}
