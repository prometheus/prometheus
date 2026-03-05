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
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	yaml "go.yaml.in/yaml/v4"

	"github.com/prometheus/prometheus/promql"
)

// Helper functions for building common structures.

// exampleTime is a reference time used for timestamp examples.
var exampleTime = time.Date(2026, 1, 2, 13, 37, 0, 0, time.UTC)

func boolPtr(b bool) *bool {
	return &b
}

func int64Ptr(i int64) *int64 {
	return &i
}

type example struct {
	name  string
	value any
}

// exampleMap creates an Examples map from the provided examples.
func exampleMap(exs []example) *orderedmap.Map[string, *base.Example] {
	examples := orderedmap.New[string, *base.Example]()
	for _, ex := range exs {
		examples.Set(ex.name, &base.Example{
			Value: createYAMLNode(ex.value),
		})
	}
	return examples
}

func schemaRef(ref string) *base.SchemaProxy {
	return base.CreateSchemaProxyRef(ref)
}

func schemaFromType(t string) *base.SchemaProxy {
	return base.CreateSchemaProxy(&base.Schema{Type: []string{t}})
}

func stringSchema() *base.SchemaProxy {
	return schemaFromType("string")
}

func integerSchema() *base.SchemaProxy {
	return base.CreateSchemaProxy(&base.Schema{
		Type:   []string{"integer"},
		Format: "int64",
	})
}

func stringSchemaWithDescription(description string) *base.SchemaProxy {
	return base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"string"},
		Description: description,
	})
}

func stringSchemaWithDescriptionAndExample(description string, example any) *base.SchemaProxy {
	return base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"string"},
		Description: description,
		Example:     createYAMLNode(example),
	})
}

func integerSchemaWithDescription(description string) *base.SchemaProxy {
	return base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"integer"},
		Format:      "int64",
		Description: description,
	})
}

func integerSchemaWithDescriptionAndExample(description string, example any) *base.SchemaProxy {
	return base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"integer"},
		Format:      "int64",
		Description: description,
		Example:     createYAMLNode(example),
	})
}

func stringArraySchemaWithDescription(description string) *base.SchemaProxy {
	return base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"array"},
		Items:       &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
		Description: description,
	})
}

func stringArraySchemaWithDescriptionAndExample(description string, example any) *base.SchemaProxy {
	return base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"array"},
		Items:       &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
		Description: description,
		Example:     createYAMLNode(example),
	})
}

func statusSchema() *base.SchemaProxy {
	successNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "success"}
	errorNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "error"}
	exampleNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "success"}
	return base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"string"},
		Enum:        []*yaml.Node{successNode, errorNode},
		Description: "Response status.",
		Example:     exampleNode,
	})
}

func warningsSchema() *base.SchemaProxy {
	return base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"array"},
		Items:       &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
		Description: "Only set if there were warnings while executing the request. There will still be data in the data field.",
	})
}

func infosSchema() *base.SchemaProxy {
	return base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"array"},
		Items:       &base.DynamicValue[*base.SchemaProxy, bool]{A: stringSchema()},
		Description: "Only set if there were info-level annotations while executing the request.",
	})
}

func timestampSchema() *base.SchemaProxy {
	return base.CreateSchemaProxy(&base.Schema{
		OneOf: []*base.SchemaProxy{
			base.CreateSchemaProxy(&base.Schema{
				Type:        []string{"string"},
				Format:      "date-time",
				Description: "RFC3339 timestamp.",
			}),
			base.CreateSchemaProxy(&base.Schema{
				Type:        []string{"number"},
				Format:      "unixtime",
				Description: "Unix timestamp in seconds.",
			}),
		},
		Description: "Timestamp in RFC3339 format or Unix timestamp in seconds.",
	})
}

func stringSchemaWithConstValue(value string) *base.SchemaProxy {
	node := &yaml.Node{Kind: yaml.ScalarNode, Value: value}
	return base.CreateSchemaProxy(&base.Schema{
		Type: []string{"string"},
		Enum: []*yaml.Node{node},
	})
}

func dateTimeSchemaWithDescription(description string) *base.SchemaProxy {
	return base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"string"},
		Format:      "date-time",
		Description: description,
	})
}

func numberSchemaWithDescription(description string) *base.SchemaProxy {
	return base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"number"},
		Format:      "double",
		Description: description,
	})
}

func errorResponse() *v3.Response {
	content := orderedmap.New[string, *v3.MediaType]()
	content.Set("application/json", &v3.MediaType{
		Schema: schemaRef("#/components/schemas/Error"),
	})
	return &v3.Response{
		Description: "Error",
		Content:     content,
	}
}

func noContentResponse() *v3.Response {
	return &v3.Response{Description: "No Content"}
}

func responsesNoContent() *v3.Responses {
	codes := orderedmap.New[string, *v3.Response]()
	codes.Set("204", noContentResponse())
	codes.Set("default", errorResponse())
	return &v3.Responses{Codes: codes}
}

func pathParam(name, description string, schema *base.SchemaProxy) *v3.Parameter {
	return &v3.Parameter{
		Name:        name,
		In:          "path",
		Description: description,
		Required:    boolPtr(true),
		Schema:      schema,
	}
}

// createYAMLNode converts Go data to yaml.Node for use in examples.
func createYAMLNode(data any) *yaml.Node {
	node := &yaml.Node{}
	bytes, _ := yaml.Marshal(data)
	_ = yaml.Unmarshal(bytes, node)
	return node
}

// formRequestBodyWithExamples creates a form-encoded request body with examples.
func formRequestBodyWithExamples(schemaRef string, examples *orderedmap.Map[string, *base.Example], description string) *v3.RequestBody {
	content := orderedmap.New[string, *v3.MediaType]()
	mediaType := &v3.MediaType{
		Schema: base.CreateSchemaProxyRef("#/components/schemas/" + schemaRef),
	}
	if examples != nil {
		mediaType.Examples = examples
	}
	content.Set("application/x-www-form-urlencoded", mediaType)
	return &v3.RequestBody{
		Required:    boolPtr(true),
		Description: description,
		Content:     content,
	}
}

// jsonResponseWithExamples creates a JSON response with examples.
func jsonResponseWithExamples(schemaRef string, examples *orderedmap.Map[string, *base.Example], description string) *v3.Response {
	content := orderedmap.New[string, *v3.MediaType]()
	mediaType := &v3.MediaType{
		Schema: base.CreateSchemaProxyRef("#/components/schemas/" + schemaRef),
	}
	if examples != nil {
		mediaType.Examples = examples
	}
	content.Set("application/json", mediaType)
	return &v3.Response{
		Description: description,
		Content:     content,
	}
}

// responsesWithErrorExamples creates responses with both success and error examples.
func responsesWithErrorExamples(okSchemaRef string, successExamples, errorExamples *orderedmap.Map[string, *base.Example], successDescription, errorDescription string) *v3.Responses {
	codes := orderedmap.New[string, *v3.Response]()
	codes.Set("200", jsonResponseWithExamples(okSchemaRef, successExamples, successDescription))
	codes.Set("default", jsonResponseWithExamples("Error", errorExamples, errorDescription))
	return &v3.Responses{Codes: codes}
}

// timestampExamples returns examples for timestamp parameters (RFC3339 and epoch).
func timestampExamples(t time.Time) []example {
	return []example{
		{"RFC3339", t.Format(time.RFC3339Nano)},
		{"epoch", t.Unix()},
	}
}

// queryParamWithExample creates a query parameter with examples.
func queryParamWithExample(name, description string, required bool, schema *base.SchemaProxy, examples []example) *v3.Parameter {
	param := &v3.Parameter{
		Name:        name,
		In:          "query",
		Description: description,
		Required:    &required,
		Explode:     boolPtr(false),
		Schema:      schema,
	}
	if len(examples) > 0 {
		param.Examples = exampleMap(examples)
	}
	return param
}

// marshalToYAMLNode marshals a value using jsoniter (production marshaling) and converts to yaml.Node.
// The result is an inline JSON representation that preserves integer types for timestamps.
func marshalToYAMLNode(v any) *yaml.Node {
	jsonAPI := jsoniter.ConfigCompatibleWithStandardLibrary
	jsonBytes, err := jsonAPI.Marshal(v)
	if err != nil {
		panic(err)
	}
	node := &yaml.Node{}
	if err := yaml.Unmarshal(jsonBytes, node); err != nil {
		panic(err)
	}
	return node
}

// vectorExample creates an example for a vector query response using production marshaling.
func vectorExample(v promql.Vector) *yaml.Node {
	type response struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string        `json:"resultType"`
			Result     promql.Vector `json:"result"`
		} `json:"data"`
	}
	resp := response{Status: "success"}
	resp.Data.ResultType = "vector"
	resp.Data.Result = v
	return marshalToYAMLNode(resp)
}

// matrixExample creates an example for a matrix query response using production marshaling.
func matrixExample(m promql.Matrix) *yaml.Node {
	type response struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string        `json:"resultType"`
			Result     promql.Matrix `json:"result"`
		} `json:"data"`
	}
	resp := response{Status: "success"}
	resp.Data.ResultType = "matrix"
	resp.Data.Result = m
	return marshalToYAMLNode(resp)
}
