// This file was automatically generated based on the contents of *.tmpl
// If you need to update this file, change the contents of those files
// (or add new ones) and run 'go generate'

package main

import "golang.org/x/tools/godoc/vfs/mapfs"

var ASTTemplates = mapfs.New(map[string]string{
	`full.go`: "package foo\n\nimport (\n	\"context\"\n	\"encoding/json\"\n	\"errors\"\n	\"net/http\"\n\n	\"github.com/go-kit/kit/endpoint\"\n	httptransport \"github.com/go-kit/kit/transport/http\"\n)\n\ntype ExampleService struct {\n}\n\ntype ExampleRequest struct {\n	I int\n	S string\n}\ntype ExampleResponse struct {\n	S   string\n	Err error\n}\n\ntype Endpoints struct {\n	ExampleEndpoint endpoint.Endpoint\n}\n\nfunc (f ExampleService) ExampleEndpoint(ctx context.Context, i int, s string) (string, error) {\n	panic(errors.New(\"not implemented\"))\n}\n\nfunc makeExampleEndpoint(f ExampleService) endpoint.Endpoint {\n	return func(ctx context.Context, request interface{}) (interface{}, error) {\n		req := request.(ExampleRequest)\n		s, err := f.ExampleEndpoint(ctx, req.I, req.S)\n		return ExampleResponse{S: s, Err: err}, nil\n	}\n}\n\nfunc inlineHandlerBuilder(m *http.ServeMux, endpoints Endpoints) {\n	m.Handle(\"/bar\", httptransport.NewServer(endpoints.ExampleEndpoint, DecodeExampleRequest, EncodeExampleResponse))\n}\n\nfunc NewHTTPHandler(endpoints Endpoints) http.Handler {\n	m := http.NewServeMux()\n	inlineHandlerBuilder(m, endpoints)\n	return m\n}\n\nfunc DecodeExampleRequest(_ context.Context, r *http.Request) (interface{}, error) {\n	var req ExampleRequest\n	err := json.NewDecoder(r.Body).Decode(&req)\n	return req, err\n}\n\nfunc EncodeExampleResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {\n	w.Header().Set(\"Content-Type\", \"application/json; charset=utf-8\")\n	return json.NewEncoder(w).Encode(response)\n}\n",
})
