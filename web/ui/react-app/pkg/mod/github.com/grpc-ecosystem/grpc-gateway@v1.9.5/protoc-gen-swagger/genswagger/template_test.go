package genswagger

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/golang/protobuf/proto"
	protodescriptor "github.com/golang/protobuf/protoc-gen-go/descriptor"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway/descriptor"
	"github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway/httprule"
	swagger_options "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger/options"
)

func crossLinkFixture(f *descriptor.File) *descriptor.File {
	for _, m := range f.Messages {
		m.File = f
	}
	for _, svc := range f.Services {
		svc.File = f
		for _, m := range svc.Methods {
			m.Service = svc
			for _, b := range m.Bindings {
				b.Method = m
				for _, param := range b.PathParams {
					param.Method = m
				}
			}
		}
	}
	return f
}

func TestMessageToQueryParameters(t *testing.T) {
	type test struct {
		MsgDescs []*protodescriptor.DescriptorProto
		Message  string
		Params   []swaggerParameterObject
	}

	tests := []test{
		{
			MsgDescs: []*protodescriptor.DescriptorProto{
				&protodescriptor.DescriptorProto{
					Name: proto.String("ExampleMessage"),
					Field: []*protodescriptor.FieldDescriptorProto{
						{
							Name:   proto.String("a"),
							Type:   protodescriptor.FieldDescriptorProto_TYPE_STRING.Enum(),
							Number: proto.Int32(1),
						},
						{
							Name:   proto.String("b"),
							Type:   protodescriptor.FieldDescriptorProto_TYPE_DOUBLE.Enum(),
							Number: proto.Int32(2),
						},
						{
							Name:   proto.String("c"),
							Type:   protodescriptor.FieldDescriptorProto_TYPE_STRING.Enum(),
							Label:  protodescriptor.FieldDescriptorProto_LABEL_REPEATED.Enum(),
							Number: proto.Int32(3),
						},
					},
				},
			},
			Message: "ExampleMessage",
			Params: []swaggerParameterObject{
				swaggerParameterObject{
					Name:     "a",
					In:       "query",
					Required: false,
					Type:     "string",
				},
				swaggerParameterObject{
					Name:     "b",
					In:       "query",
					Required: false,
					Type:     "number",
					Format:   "double",
				},
				swaggerParameterObject{
					Name:             "c",
					In:               "query",
					Required:         false,
					Type:             "array",
					CollectionFormat: "multi",
				},
			},
		},
		{
			MsgDescs: []*protodescriptor.DescriptorProto{
				&protodescriptor.DescriptorProto{
					Name: proto.String("ExampleMessage"),
					Field: []*protodescriptor.FieldDescriptorProto{
						{
							Name:     proto.String("nested"),
							Type:     protodescriptor.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
							TypeName: proto.String(".example.Nested"),
							Number:   proto.Int32(1),
						},
					},
				},
				&protodescriptor.DescriptorProto{
					Name: proto.String("Nested"),
					Field: []*protodescriptor.FieldDescriptorProto{
						{
							Name:   proto.String("a"),
							Type:   protodescriptor.FieldDescriptorProto_TYPE_STRING.Enum(),
							Number: proto.Int32(1),
						},
						{
							Name:     proto.String("deep"),
							Type:     protodescriptor.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
							TypeName: proto.String(".example.Nested.DeepNested"),
							Number:   proto.Int32(2),
						},
					},
					NestedType: []*protodescriptor.DescriptorProto{{
						Name: proto.String("DeepNested"),
						Field: []*protodescriptor.FieldDescriptorProto{
							{
								Name:   proto.String("b"),
								Type:   protodescriptor.FieldDescriptorProto_TYPE_STRING.Enum(),
								Number: proto.Int32(1),
							},
							{
								Name:     proto.String("c"),
								Type:     protodescriptor.FieldDescriptorProto_TYPE_ENUM.Enum(),
								TypeName: proto.String(".example.Nested.DeepNested.DeepEnum"),
								Number:   proto.Int32(2),
							},
						},
						EnumType: []*protodescriptor.EnumDescriptorProto{
							{
								Name: proto.String("DeepEnum"),
								Value: []*protodescriptor.EnumValueDescriptorProto{
									{Name: proto.String("FALSE"), Number: proto.Int32(0)},
									{Name: proto.String("TRUE"), Number: proto.Int32(1)},
								},
							},
						},
					}},
				},
			},
			Message: "ExampleMessage",
			Params: []swaggerParameterObject{
				swaggerParameterObject{
					Name:     "nested.a",
					In:       "query",
					Required: false,
					Type:     "string",
				},
				swaggerParameterObject{
					Name:     "nested.deep.b",
					In:       "query",
					Required: false,
					Type:     "string",
				},
				swaggerParameterObject{
					Name:     "nested.deep.c",
					In:       "query",
					Required: false,
					Type:     "string",
					Enum:     []string{"FALSE", "TRUE"},
					Default:  "FALSE",
				},
			},
		},
	}

	for _, test := range tests {
		reg := descriptor.NewRegistry()
		msgs := []*descriptor.Message{}
		for _, msgdesc := range test.MsgDescs {
			msgs = append(msgs, &descriptor.Message{DescriptorProto: msgdesc})
		}
		file := descriptor.File{
			FileDescriptorProto: &protodescriptor.FileDescriptorProto{
				SourceCodeInfo: &protodescriptor.SourceCodeInfo{},
				Name:           proto.String("example.proto"),
				Package:        proto.String("example"),
				Dependency:     []string{},
				MessageType:    test.MsgDescs,
				Service:        []*protodescriptor.ServiceDescriptorProto{},
			},
			GoPkg: descriptor.GoPackage{
				Path: "example.com/path/to/example/example.pb",
				Name: "example_pb",
			},
			Messages: msgs,
		}
		reg.Load(&plugin.CodeGeneratorRequest{
			ProtoFile: []*protodescriptor.FileDescriptorProto{file.FileDescriptorProto},
		})

		message, err := reg.LookupMsg("", ".example."+test.Message)
		if err != nil {
			t.Fatalf("failed to lookup message: %s", err)
		}
		params, err := messageToQueryParameters(message, reg, []descriptor.Parameter{})
		if err != nil {
			t.Fatalf("failed to convert message to query parameters: %s", err)
		}
		// avoid checking Items for array types
		for i := range params {
			params[i].Items = nil
		}
		if !reflect.DeepEqual(params, test.Params) {
			t.Errorf("expected %v, got %v", test.Params, params)
		}
	}
}

func TestMessageToQueryParametersWithJsonName(t *testing.T) {
	type test struct {
		MsgDescs []*protodescriptor.DescriptorProto
		Message  string
		Params   []swaggerParameterObject
	}

	tests := []test{
		{
			MsgDescs: []*protodescriptor.DescriptorProto{
				&protodescriptor.DescriptorProto{
					Name: proto.String("ExampleMessage"),
					Field: []*protodescriptor.FieldDescriptorProto{
						{
							Name:     proto.String("test_field_a"),
							Type:     protodescriptor.FieldDescriptorProto_TYPE_STRING.Enum(),
							Number:   proto.Int32(1),
							JsonName: proto.String("testFieldA"),
						},
					},
				},
			},
			Message: "ExampleMessage",
			Params: []swaggerParameterObject{
				swaggerParameterObject{
					Name:     "testFieldA",
					In:       "query",
					Required: false,
					Type:     "string",
				},
			},
		},
	}

	for _, test := range tests {
		reg := descriptor.NewRegistry()
		reg.SetUseJSONNamesForFields(true)
		msgs := []*descriptor.Message{}
		for _, msgdesc := range test.MsgDescs {
			msgs = append(msgs, &descriptor.Message{DescriptorProto: msgdesc})
		}
		file := descriptor.File{
			FileDescriptorProto: &protodescriptor.FileDescriptorProto{
				SourceCodeInfo: &protodescriptor.SourceCodeInfo{},
				Name:           proto.String("example.proto"),
				Package:        proto.String("example"),
				Dependency:     []string{},
				MessageType:    test.MsgDescs,
				Service:        []*protodescriptor.ServiceDescriptorProto{},
			},
			GoPkg: descriptor.GoPackage{
				Path: "example.com/path/to/example/example.pb",
				Name: "example_pb",
			},
			Messages: msgs,
		}
		reg.Load(&plugin.CodeGeneratorRequest{
			ProtoFile: []*protodescriptor.FileDescriptorProto{file.FileDescriptorProto},
		})

		message, err := reg.LookupMsg("", ".example."+test.Message)
		if err != nil {
			t.Fatalf("failed to lookup message: %s", err)
		}
		params, err := messageToQueryParameters(message, reg, []descriptor.Parameter{})
		if err != nil {
			t.Fatalf("failed to convert message to query parameters: %s", err)
		}
		if !reflect.DeepEqual(params, test.Params) {
			t.Errorf("expected %v, got %v", test.Params, params)
		}
	}
}

func TestApplyTemplateSimple(t *testing.T) {
	msgdesc := &protodescriptor.DescriptorProto{
		Name: proto.String("ExampleMessage"),
	}
	meth := &protodescriptor.MethodDescriptorProto{
		Name:       proto.String("Example"),
		InputType:  proto.String("ExampleMessage"),
		OutputType: proto.String("ExampleMessage"),
	}
	svc := &protodescriptor.ServiceDescriptorProto{
		Name:   proto.String("ExampleService"),
		Method: []*protodescriptor.MethodDescriptorProto{meth},
	}
	msg := &descriptor.Message{
		DescriptorProto: msgdesc,
	}
	file := descriptor.File{
		FileDescriptorProto: &protodescriptor.FileDescriptorProto{
			SourceCodeInfo: &protodescriptor.SourceCodeInfo{},
			Name:           proto.String("example.proto"),
			Package:        proto.String("example"),
			Dependency:     []string{"a.example/b/c.proto", "a.example/d/e.proto"},
			MessageType:    []*protodescriptor.DescriptorProto{msgdesc},
			Service:        []*protodescriptor.ServiceDescriptorProto{svc},
		},
		GoPkg: descriptor.GoPackage{
			Path: "example.com/path/to/example/example.pb",
			Name: "example_pb",
		},
		Messages: []*descriptor.Message{msg},
		Services: []*descriptor.Service{
			{
				ServiceDescriptorProto: svc,
				Methods: []*descriptor.Method{
					{
						MethodDescriptorProto: meth,
						RequestType:           msg,
						ResponseType:          msg,
						Bindings: []*descriptor.Binding{
							{
								HTTPMethod: "GET",
								Body:       &descriptor.Body{FieldPath: nil},
								PathTmpl: httprule.Template{
									Version:  1,
									OpCodes:  []int{0, 0},
									Template: "/v1/echo", // TODO(achew22): Figure out what this should really be
								},
							},
						},
					},
				},
			},
		},
	}
	result, err := applyTemplate(param{File: crossLinkFixture(&file), reg: descriptor.NewRegistry()})
	if err != nil {
		t.Errorf("applyTemplate(%#v) failed with %v; want success", file, err)
		return
	}
	if want, is, name := "2.0", result.Swagger, "Swagger"; !reflect.DeepEqual(is, want) {
		t.Errorf("applyTemplate(%#v).%s = %s want to be %s", file, name, is, want)
	}
	if want, is, name := "", result.BasePath, "BasePath"; !reflect.DeepEqual(is, want) {
		t.Errorf("applyTemplate(%#v).%s = %s want to be %s", file, name, is, want)
	}
	if want, is, name := []string{"http", "https"}, result.Schemes, "Schemes"; !reflect.DeepEqual(is, want) {
		t.Errorf("applyTemplate(%#v).%s = %s want to be %s", file, name, is, want)
	}
	if want, is, name := []string{"application/json"}, result.Consumes, "Consumes"; !reflect.DeepEqual(is, want) {
		t.Errorf("applyTemplate(%#v).%s = %s want to be %s", file, name, is, want)
	}
	if want, is, name := []string{"application/json"}, result.Produces, "Produces"; !reflect.DeepEqual(is, want) {
		t.Errorf("applyTemplate(%#v).%s = %s want to be %s", file, name, is, want)
	}

	// If there was a failure, print out the input and the json result for debugging.
	if t.Failed() {
		t.Errorf("had: %s", file)
		t.Errorf("got: %s", fmt.Sprint(result))
	}
}

func TestApplyTemplateRequestWithoutClientStreaming(t *testing.T) {
	msgdesc := &protodescriptor.DescriptorProto{
		Name: proto.String("ExampleMessage"),
		Field: []*protodescriptor.FieldDescriptorProto{
			{
				Name:     proto.String("nested"),
				Label:    protodescriptor.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:     protodescriptor.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				TypeName: proto.String("NestedMessage"),
				Number:   proto.Int32(1),
			},
		},
	}
	nesteddesc := &protodescriptor.DescriptorProto{
		Name: proto.String("NestedMessage"),
		Field: []*protodescriptor.FieldDescriptorProto{
			{
				Name:   proto.String("int32"),
				Label:  protodescriptor.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   protodescriptor.FieldDescriptorProto_TYPE_INT32.Enum(),
				Number: proto.Int32(1),
			},
			{
				Name:   proto.String("bool"),
				Label:  protodescriptor.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   protodescriptor.FieldDescriptorProto_TYPE_BOOL.Enum(),
				Number: proto.Int32(2),
			},
		},
	}
	meth := &protodescriptor.MethodDescriptorProto{
		Name:            proto.String("Echo"),
		InputType:       proto.String("ExampleMessage"),
		OutputType:      proto.String("ExampleMessage"),
		ClientStreaming: proto.Bool(false),
	}
	svc := &protodescriptor.ServiceDescriptorProto{
		Name:   proto.String("ExampleService"),
		Method: []*protodescriptor.MethodDescriptorProto{meth},
	}

	meth.ServerStreaming = proto.Bool(false)

	msg := &descriptor.Message{
		DescriptorProto: msgdesc,
	}
	nested := &descriptor.Message{
		DescriptorProto: nesteddesc,
	}

	nestedField := &descriptor.Field{
		Message:              msg,
		FieldDescriptorProto: msg.GetField()[0],
	}
	intField := &descriptor.Field{
		Message:              nested,
		FieldDescriptorProto: nested.GetField()[0],
	}
	boolField := &descriptor.Field{
		Message:              nested,
		FieldDescriptorProto: nested.GetField()[1],
	}
	file := descriptor.File{
		FileDescriptorProto: &protodescriptor.FileDescriptorProto{
			SourceCodeInfo: &protodescriptor.SourceCodeInfo{},
			Name:           proto.String("example.proto"),
			Package:        proto.String("example"),
			MessageType:    []*protodescriptor.DescriptorProto{msgdesc, nesteddesc},
			Service:        []*protodescriptor.ServiceDescriptorProto{svc},
		},
		GoPkg: descriptor.GoPackage{
			Path: "example.com/path/to/example/example.pb",
			Name: "example_pb",
		},
		Messages: []*descriptor.Message{msg, nested},
		Services: []*descriptor.Service{
			{
				ServiceDescriptorProto: svc,
				Methods: []*descriptor.Method{
					{
						MethodDescriptorProto: meth,
						RequestType:           msg,
						ResponseType:          msg,
						Bindings: []*descriptor.Binding{
							{
								HTTPMethod: "POST",
								PathTmpl: httprule.Template{
									Version:  1,
									OpCodes:  []int{0, 0},
									Template: "/v1/echo", // TODO(achew): Figure out what this hsould really be
								},
								PathParams: []descriptor.Parameter{
									{
										FieldPath: descriptor.FieldPath([]descriptor.FieldPathComponent{
											{
												Name:   "nested",
												Target: nestedField,
											},
											{
												Name:   "int32",
												Target: intField,
											},
										}),
										Target: intField,
									},
								},
								Body: &descriptor.Body{
									FieldPath: descriptor.FieldPath([]descriptor.FieldPathComponent{
										{
											Name:   "nested",
											Target: nestedField,
										},
										{
											Name:   "bool",
											Target: boolField,
										},
									}),
								},
							},
						},
					},
				},
			},
		},
	}
	reg := descriptor.NewRegistry()
	reg.Load(&plugin.CodeGeneratorRequest{ProtoFile: []*protodescriptor.FileDescriptorProto{file.FileDescriptorProto}})
	result, err := applyTemplate(param{File: crossLinkFixture(&file), reg: reg})
	if err != nil {
		t.Errorf("applyTemplate(%#v) failed with %v; want success", file, err)
		return
	}
	if want, got := "2.0", result.Swagger; !reflect.DeepEqual(got, want) {
		t.Errorf("applyTemplate(%#v).Swagger = %s want to be %s", file, got, want)
	}
	if want, got := "", result.BasePath; !reflect.DeepEqual(got, want) {
		t.Errorf("applyTemplate(%#v).BasePath = %s want to be %s", file, got, want)
	}
	if want, got := []string{"http", "https"}, result.Schemes; !reflect.DeepEqual(got, want) {
		t.Errorf("applyTemplate(%#v).Schemes = %s want to be %s", file, got, want)
	}
	if want, got := []string{"application/json"}, result.Consumes; !reflect.DeepEqual(got, want) {
		t.Errorf("applyTemplate(%#v).Consumes = %s want to be %s", file, got, want)
	}
	if want, got := []string{"application/json"}, result.Produces; !reflect.DeepEqual(got, want) {
		t.Errorf("applyTemplate(%#v).Produces = %s want to be %s", file, got, want)
	}

	// If there was a failure, print out the input and the json result for debugging.
	if t.Failed() {
		t.Errorf("had: %s", file)
		t.Errorf("got: %s", fmt.Sprint(result))
	}
}

func TestApplyTemplateRequestWithClientStreaming(t *testing.T) {
	msgdesc := &protodescriptor.DescriptorProto{
		Name: proto.String("ExampleMessage"),
		Field: []*protodescriptor.FieldDescriptorProto{
			{
				Name:     proto.String("nested"),
				Label:    protodescriptor.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:     protodescriptor.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				TypeName: proto.String("NestedMessage"),
				Number:   proto.Int32(1),
			},
		},
	}
	nesteddesc := &protodescriptor.DescriptorProto{
		Name: proto.String("NestedMessage"),
		Field: []*protodescriptor.FieldDescriptorProto{
			{
				Name:   proto.String("int32"),
				Label:  protodescriptor.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   protodescriptor.FieldDescriptorProto_TYPE_INT32.Enum(),
				Number: proto.Int32(1),
			},
			{
				Name:   proto.String("bool"),
				Label:  protodescriptor.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   protodescriptor.FieldDescriptorProto_TYPE_BOOL.Enum(),
				Number: proto.Int32(2),
			},
		},
	}
	meth := &protodescriptor.MethodDescriptorProto{
		Name:            proto.String("Echo"),
		InputType:       proto.String("ExampleMessage"),
		OutputType:      proto.String("ExampleMessage"),
		ClientStreaming: proto.Bool(true),
		ServerStreaming: proto.Bool(true),
	}
	svc := &protodescriptor.ServiceDescriptorProto{
		Name:   proto.String("ExampleService"),
		Method: []*protodescriptor.MethodDescriptorProto{meth},
	}

	msg := &descriptor.Message{
		DescriptorProto: msgdesc,
	}
	nested := &descriptor.Message{
		DescriptorProto: nesteddesc,
	}

	nestedField := &descriptor.Field{
		Message:              msg,
		FieldDescriptorProto: msg.GetField()[0],
	}
	intField := &descriptor.Field{
		Message:              nested,
		FieldDescriptorProto: nested.GetField()[0],
	}
	boolField := &descriptor.Field{
		Message:              nested,
		FieldDescriptorProto: nested.GetField()[1],
	}
	file := descriptor.File{
		FileDescriptorProto: &protodescriptor.FileDescriptorProto{
			SourceCodeInfo: &protodescriptor.SourceCodeInfo{},
			Name:           proto.String("example.proto"),
			Package:        proto.String("example"),
			MessageType:    []*protodescriptor.DescriptorProto{msgdesc, nesteddesc},
			Service:        []*protodescriptor.ServiceDescriptorProto{svc},
		},
		GoPkg: descriptor.GoPackage{
			Path: "example.com/path/to/example/example.pb",
			Name: "example_pb",
		},
		Messages: []*descriptor.Message{msg, nested},
		Services: []*descriptor.Service{
			{
				ServiceDescriptorProto: svc,
				Methods: []*descriptor.Method{
					{
						MethodDescriptorProto: meth,
						RequestType:           msg,
						ResponseType:          msg,
						Bindings: []*descriptor.Binding{
							{
								HTTPMethod: "POST",
								PathTmpl: httprule.Template{
									Version:  1,
									OpCodes:  []int{0, 0},
									Template: "/v1/echo", // TODO(achew): Figure out what this hsould really be
								},
								PathParams: []descriptor.Parameter{
									{
										FieldPath: descriptor.FieldPath([]descriptor.FieldPathComponent{
											{
												Name:   "nested",
												Target: nestedField,
											},
											{
												Name:   "int32",
												Target: intField,
											},
										}),
										Target: intField,
									},
								},
								Body: &descriptor.Body{
									FieldPath: descriptor.FieldPath([]descriptor.FieldPathComponent{
										{
											Name:   "nested",
											Target: nestedField,
										},
										{
											Name:   "bool",
											Target: boolField,
										},
									}),
								},
							},
						},
					},
				},
			},
		},
	}
	reg := descriptor.NewRegistry()
	if err := AddStreamError(reg); err != nil {
		t.Errorf("AddStreamError(%#v) failed with %v; want success", reg, err)
		return
	}
	reg.Load(&plugin.CodeGeneratorRequest{ProtoFile: []*protodescriptor.FileDescriptorProto{file.FileDescriptorProto}})
	result, err := applyTemplate(param{File: crossLinkFixture(&file), reg: reg})
	if err != nil {
		t.Errorf("applyTemplate(%#v) failed with %v; want success", file, err)
		return
	}

	// Only ExampleMessage must be present, not NestedMessage
	if want, got, name := 3, len(result.Definitions), "len(Definitions)"; !reflect.DeepEqual(got, want) {
		t.Errorf("applyTemplate(%#v).%s = %d want to be %d", file, name, got, want)
	}
	// stream ExampleMessage must be present
	if want, got, name := 1, len(result.StreamDefinitions), "len(StreamDefinitions)"; !reflect.DeepEqual(got, want) {
		t.Errorf("applyTemplate(%#v).%s = %d want to be %d", file, name, got, want)
	} else {
		streamExampleExampleMessage := result.StreamDefinitions["exampleExampleMessage"]
		if want, got, name := "object", streamExampleExampleMessage.Type, `StreamDefinitions["exampleExampleMessage"].Type`; !reflect.DeepEqual(got, want) {
			t.Errorf("applyTemplate(%#v).%s = %s want to be %s", file, name, got, want)
		}
		if want, got, name := "Stream result of exampleExampleMessage", streamExampleExampleMessage.Title, `StreamDefinitions["exampleExampleMessage"].Title`; !reflect.DeepEqual(got, want) {
			t.Errorf("applyTemplate(%#v).%s = %s want to be %s", file, name, got, want)
		}
		streamExampleExampleMessageProperties := *(streamExampleExampleMessage.Properties)
		if want, got, name := 2, len(streamExampleExampleMessageProperties), `len(StreamDefinitions["exampleExampleMessage"].Properties)`; !reflect.DeepEqual(got, want) {
			t.Errorf("applyTemplate(%#v).%s = %d want to be %d", file, name, got, want)
		} else {
			resultProperty := streamExampleExampleMessageProperties[0]
			if want, got, name := "result", resultProperty.Key, `(*(StreamDefinitions["exampleExampleMessage"].Properties))[0].Key`; !reflect.DeepEqual(got, want) {
				t.Errorf("applyTemplate(%#v).%s = %s want to be %s", file, name, got, want)
			}
			result := resultProperty.Value.(swaggerSchemaObject)
			if want, got, name := "#/definitions/exampleExampleMessage", result.Ref, `((*(StreamDefinitions["exampleExampleMessage"].Properties))[0].Value.(swaggerSchemaObject)).Ref`; !reflect.DeepEqual(got, want) {
				t.Errorf("applyTemplate(%#v).%s = %s want to be %s", file, name, got, want)
			}
			errorProperty := streamExampleExampleMessageProperties[1]
			if want, got, name := "error", errorProperty.Key, `(*(StreamDefinitions["exampleExampleMessage"].Properties))[0].Key`; !reflect.DeepEqual(got, want) {
				t.Errorf("applyTemplate(%#v).%s = %s want to be %s", file, name, got, want)
			}
			err := errorProperty.Value.(swaggerSchemaObject)
			if want, got, name := "#/definitions/runtimeStreamError", err.Ref, `((*(StreamDefinitions["exampleExampleMessage"].Properties))[0].Value.(swaggerSchemaObject)).Ref`; !reflect.DeepEqual(got, want) {
				t.Errorf("applyTemplate(%#v).%s = %s want to be %s", file, name, got, want)
			}
		}
	}
	if want, got, name := 1, len(result.Paths["/v1/echo"].Post.Responses), "len(Paths[/v1/echo].Post.Responses)"; !reflect.DeepEqual(got, want) {
		t.Errorf("applyTemplate(%#v).%s = %d want to be %d", file, name, got, want)
	} else {
		if want, got, name := "A successful response.(streaming responses)", result.Paths["/v1/echo"].Post.Responses["200"].Description, `result.Paths["/v1/echo"].Post.Responses["200"].Description`; !reflect.DeepEqual(got, want) {
			t.Errorf("applyTemplate(%#v).%s = %s want to be %s", file, name, got, want)
		}
		if want, got, name := "#/x-stream-definitions/exampleExampleMessage", result.Paths["/v1/echo"].Post.Responses["200"].Schema.Ref, `result.Paths["/v1/echo"].Post.Responses["200"].Description`; !reflect.DeepEqual(got, want) {
			t.Errorf("applyTemplate(%#v).%s = %s want to be %s", file, name, got, want)
		}
	}

	// If there was a failure, print out the input and the json result for debugging.
	if t.Failed() {
		t.Errorf("had: %s", file)
		t.Errorf("got: %s", fmt.Sprint(result))
	}
}

func TestApplyTemplateRequestWithUnusedReferences(t *testing.T) {
	reqdesc := &protodescriptor.DescriptorProto{
		Name: proto.String("ExampleMessage"),
		Field: []*protodescriptor.FieldDescriptorProto{
			{
				Name:   proto.String("string"),
				Label:  protodescriptor.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   protodescriptor.FieldDescriptorProto_TYPE_STRING.Enum(),
				Number: proto.Int32(1),
			},
		},
	}
	respdesc := &protodescriptor.DescriptorProto{
		Name: proto.String("EmptyMessage"),
	}
	meth := &protodescriptor.MethodDescriptorProto{
		Name:            proto.String("Example"),
		InputType:       proto.String("ExampleMessage"),
		OutputType:      proto.String("EmptyMessage"),
		ClientStreaming: proto.Bool(false),
		ServerStreaming: proto.Bool(false),
	}
	svc := &protodescriptor.ServiceDescriptorProto{
		Name:   proto.String("ExampleService"),
		Method: []*protodescriptor.MethodDescriptorProto{meth},
	}

	req := &descriptor.Message{
		DescriptorProto: reqdesc,
	}
	resp := &descriptor.Message{
		DescriptorProto: respdesc,
	}
	stringField := &descriptor.Field{
		Message:              req,
		FieldDescriptorProto: req.GetField()[0],
	}
	file := descriptor.File{
		FileDescriptorProto: &protodescriptor.FileDescriptorProto{
			SourceCodeInfo: &protodescriptor.SourceCodeInfo{},
			Name:           proto.String("example.proto"),
			Package:        proto.String("example"),
			MessageType:    []*protodescriptor.DescriptorProto{reqdesc, respdesc},
			Service:        []*protodescriptor.ServiceDescriptorProto{svc},
		},
		GoPkg: descriptor.GoPackage{
			Path: "example.com/path/to/example/example.pb",
			Name: "example_pb",
		},
		Messages: []*descriptor.Message{req, resp},
		Services: []*descriptor.Service{
			{
				ServiceDescriptorProto: svc,
				Methods: []*descriptor.Method{
					{
						MethodDescriptorProto: meth,
						RequestType:           req,
						ResponseType:          resp,
						Bindings: []*descriptor.Binding{
							{
								HTTPMethod: "GET",
								PathTmpl: httprule.Template{
									Version:  1,
									OpCodes:  []int{0, 0},
									Template: "/v1/example",
								},
							},
							{
								HTTPMethod: "POST",
								PathTmpl: httprule.Template{
									Version:  1,
									OpCodes:  []int{0, 0},
									Template: "/v1/example/{string}",
								},
								PathParams: []descriptor.Parameter{
									{
										FieldPath: descriptor.FieldPath([]descriptor.FieldPathComponent{
											{
												Name:   "string",
												Target: stringField,
											},
										}),
										Target: stringField,
									},
								},
								Body: &descriptor.Body{
									FieldPath: descriptor.FieldPath([]descriptor.FieldPathComponent{
										{
											Name:   "string",
											Target: stringField,
										},
									}),
								},
							},
						},
					},
				},
			},
		},
	}

	reg := descriptor.NewRegistry()
	reg.Load(&plugin.CodeGeneratorRequest{ProtoFile: []*protodescriptor.FileDescriptorProto{file.FileDescriptorProto}})
	result, err := applyTemplate(param{File: crossLinkFixture(&file), reg: reg})
	if err != nil {
		t.Errorf("applyTemplate(%#v) failed with %v; want success", file, err)
		return
	}

	// Only EmptyMessage must be present, not ExampleMessage
	if want, got, name := 1, len(result.Definitions), "len(Definitions)"; !reflect.DeepEqual(got, want) {
		t.Errorf("applyTemplate(%#v).%s = %d want to be %d", file, name, got, want)
	}

	// If there was a failure, print out the input and the json result for debugging.
	if t.Failed() {
		t.Errorf("had: %s", file)
		t.Errorf("got: %s", fmt.Sprint(result))
	}
}

func TestTemplateToSwaggerPath(t *testing.T) {
	var tests = []struct {
		input    string
		expected string
	}{
		{"/test", "/test"},
		{"/{test}", "/{test}"},
		{"/{test=prefix/*}", "/{test}"},
		{"/{test=prefix/that/has/multiple/parts/to/it/*}", "/{test}"},
		{"/{test1}/{test2}", "/{test1}/{test2}"},
		{"/{test1}/{test2}/", "/{test1}/{test2}/"},
		{"/{name=prefix/*}", "/{name=prefix/*}"},
		{"/{name=prefix1/*/prefix2/*}", "/{name=prefix1/*/prefix2/*}"},
		{"/{user.name=prefix/*}", "/{user.name=prefix/*}"},
		{"/{user.name=prefix1/*/prefix2/*}", "/{user.name=prefix1/*/prefix2/*}"},
		{"/{parent=prefix/*}/children", "/{parent=prefix/*}/children"},
		{"/{name=prefix/*}:customMethod", "/{name=prefix/*}:customMethod"},
		{"/{name=prefix1/*/prefix2/*}:customMethod", "/{name=prefix1/*/prefix2/*}:customMethod"},
		{"/{user.name=prefix/*}:customMethod", "/{user.name=prefix/*}:customMethod"},
		{"/{user.name=prefix1/*/prefix2/*}:customMethod", "/{user.name=prefix1/*/prefix2/*}:customMethod"},
		{"/{parent=prefix/*}/children:customMethod", "/{parent=prefix/*}/children:customMethod"},
	}

	for _, data := range tests {
		actual := templateToSwaggerPath(data.input)
		if data.expected != actual {
			t.Errorf("Expected templateToSwaggerPath(%v) = %v, actual: %v", data.input, data.expected, actual)
		}
	}
}

func TestResolveFullyQualifiedNameToSwaggerName(t *testing.T) {
	var tests = []struct {
		input                string
		output               string
		listOfFQMNs          []string
		useFQNForSwaggerName bool
	}{
		{
			".a.b.C",
			"C",
			[]string{
				".a.b.C",
			},
			false,
		},
		{
			".a.b.C",
			"abC",
			[]string{
				".a.C",
				".a.b.C",
			},
			false,
		},
		{
			".a.b.C",
			"abC",
			[]string{
				".C",
				".a.C",
				".a.b.C",
			},
			false,
		},
		{
			".a.b.C",
			"a.b.C",
			[]string{
				".C",
				".a.C",
				".a.b.C",
			},
			true,
		},
	}

	for _, data := range tests {
		names := resolveFullyQualifiedNameToSwaggerNames(data.listOfFQMNs, data.useFQNForSwaggerName)
		output := names[data.input]
		if output != data.output {
			t.Errorf("Expected fullyQualifiedNameToSwaggerName(%v) to be %s but got %s",
				data.input, data.output, output)
		}
	}
}

func TestFQMNtoSwaggerName(t *testing.T) {
	var tests = []struct {
		input    string
		expected string
	}{
		{"/test", "/test"},
		{"/{test}", "/{test}"},
		{"/{test=prefix/*}", "/{test}"},
		{"/{test=prefix/that/has/multiple/parts/to/it/*}", "/{test}"},
		{"/{test1}/{test2}", "/{test1}/{test2}"},
		{"/{test1}/{test2}/", "/{test1}/{test2}/"},
	}

	for _, data := range tests {
		actual := templateToSwaggerPath(data.input)
		if data.expected != actual {
			t.Errorf("Expected templateToSwaggerPath(%v) = %v, actual: %v", data.input, data.expected, actual)
		}
	}
}

func TestSchemaOfField(t *testing.T) {
	type test struct {
		field    *descriptor.Field
		refs     refMap
		expected schemaCore
	}

	tests := []test{
		{
			field: &descriptor.Field{
				FieldDescriptorProto: &protodescriptor.FieldDescriptorProto{
					Name: proto.String("primitive_field"),
					Type: protodescriptor.FieldDescriptorProto_TYPE_STRING.Enum(),
				},
			},
			refs: make(refMap),
			expected: schemaCore{
				Type: "string",
			},
		},
		{
			field: &descriptor.Field{
				FieldDescriptorProto: &protodescriptor.FieldDescriptorProto{
					Name:  proto.String("repeated_primitive_field"),
					Type:  protodescriptor.FieldDescriptorProto_TYPE_STRING.Enum(),
					Label: protodescriptor.FieldDescriptorProto_LABEL_REPEATED.Enum(),
				},
			},
			refs: make(refMap),
			expected: schemaCore{
				Type: "array",
				Items: &swaggerItemsObject{
					Type: "string",
				},
			},
		},
		{
			field: &descriptor.Field{
				FieldDescriptorProto: &protodescriptor.FieldDescriptorProto{
					Name:     proto.String("wrapped_field"),
					TypeName: proto.String(".google.protobuf.StringValue"),
					Type:     protodescriptor.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				},
			},
			refs: make(refMap),
			expected: schemaCore{
				Type: "string",
			},
		},
		{
			field: &descriptor.Field{
				FieldDescriptorProto: &protodescriptor.FieldDescriptorProto{
					Name:     proto.String("repeated_wrapped_field"),
					TypeName: proto.String(".google.protobuf.StringValue"),
					Type:     protodescriptor.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					Label:    protodescriptor.FieldDescriptorProto_LABEL_REPEATED.Enum(),
				},
			},
			refs: make(refMap),
			expected: schemaCore{
				Type: "array",
				Items: &swaggerItemsObject{
					Type: "string",
				},
			},
		},
		{
			field: &descriptor.Field{
				FieldDescriptorProto: &protodescriptor.FieldDescriptorProto{
					Name:     proto.String("wrapped_field"),
					TypeName: proto.String(".google.protobuf.BytesValue"),
					Type:     protodescriptor.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				},
			},
			refs: make(refMap),
			expected: schemaCore{
				Type:   "string",
				Format: "byte",
			},
		},
		{
			field: &descriptor.Field{
				FieldDescriptorProto: &protodescriptor.FieldDescriptorProto{
					Name:     proto.String("wrapped_field"),
					TypeName: proto.String(".google.protobuf.Int32Value"),
					Type:     protodescriptor.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				},
			},
			refs: make(refMap),
			expected: schemaCore{
				Type:   "integer",
				Format: "int32",
			},
		},
		{
			field: &descriptor.Field{
				FieldDescriptorProto: &protodescriptor.FieldDescriptorProto{
					Name:     proto.String("wrapped_field"),
					TypeName: proto.String(".google.protobuf.UInt32Value"),
					Type:     protodescriptor.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				},
			},
			refs: make(refMap),
			expected: schemaCore{
				Type:   "integer",
				Format: "int64",
			},
		},
		{
			field: &descriptor.Field{
				FieldDescriptorProto: &protodescriptor.FieldDescriptorProto{
					Name:     proto.String("wrapped_field"),
					TypeName: proto.String(".google.protobuf.Int64Value"),
					Type:     protodescriptor.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				},
			},
			refs: make(refMap),
			expected: schemaCore{
				Type:   "string",
				Format: "int64",
			},
		},
		{
			field: &descriptor.Field{
				FieldDescriptorProto: &protodescriptor.FieldDescriptorProto{
					Name:     proto.String("wrapped_field"),
					TypeName: proto.String(".google.protobuf.UInt64Value"),
					Type:     protodescriptor.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				},
			},
			refs: make(refMap),
			expected: schemaCore{
				Type:   "string",
				Format: "uint64",
			},
		},
		{
			field: &descriptor.Field{
				FieldDescriptorProto: &protodescriptor.FieldDescriptorProto{
					Name:     proto.String("wrapped_field"),
					TypeName: proto.String(".google.protobuf.FloatValue"),
					Type:     protodescriptor.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				},
			},
			refs: make(refMap),
			expected: schemaCore{
				Type:   "number",
				Format: "float",
			},
		},
		{
			field: &descriptor.Field{
				FieldDescriptorProto: &protodescriptor.FieldDescriptorProto{
					Name:     proto.String("wrapped_field"),
					TypeName: proto.String(".google.protobuf.DoubleValue"),
					Type:     protodescriptor.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				},
			},
			refs: make(refMap),
			expected: schemaCore{
				Type:   "number",
				Format: "double",
			},
		},
		{
			field: &descriptor.Field{
				FieldDescriptorProto: &protodescriptor.FieldDescriptorProto{
					Name:     proto.String("wrapped_field"),
					TypeName: proto.String(".google.protobuf.BoolValue"),
					Type:     protodescriptor.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				},
			},
			refs: make(refMap),
			expected: schemaCore{
				Type:   "boolean",
				Format: "boolean",
			},
		},
		{
			field: &descriptor.Field{
				FieldDescriptorProto: &protodescriptor.FieldDescriptorProto{
					Name:     proto.String("message_field"),
					TypeName: proto.String(".example.Message"),
					Type:     protodescriptor.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				},
			},
			refs: refMap{".example.Message": struct{}{}},
			expected: schemaCore{
				Ref: "#/definitions/exampleMessage",
			},
		},
	}

	reg := descriptor.NewRegistry()
	reg.Load(&plugin.CodeGeneratorRequest{
		ProtoFile: []*protodescriptor.FileDescriptorProto{
			{
				SourceCodeInfo: &protodescriptor.SourceCodeInfo{},
				Name:           proto.String("example.proto"),
				Package:        proto.String("example"),
				Dependency:     []string{},
				MessageType: []*protodescriptor.DescriptorProto{
					{
						Name: proto.String("Message"),
						Field: []*protodescriptor.FieldDescriptorProto{
							{
								Name: proto.String("value"),
								Type: protodescriptor.FieldDescriptorProto_TYPE_STRING.Enum(),
							},
						},
					},
				},
				EnumType: []*protodescriptor.EnumDescriptorProto{
					{
						Name: proto.String("Message"),
					},
				},
				Service: []*protodescriptor.ServiceDescriptorProto{},
			},
		},
	})

	for _, test := range tests {
		refs := make(refMap)
		actual := schemaOfField(test.field, reg, refs)
		expectedSchemaObject := swaggerSchemaObject{schemaCore: test.expected}
		if e, a := expectedSchemaObject, actual; !reflect.DeepEqual(a, e) {
			t.Errorf("Expected schemaOfField(%v) = %v, actual: %v", test.field, e, a)
		}
		if !reflect.DeepEqual(refs, test.refs) {
			t.Errorf("Expected schemaOfField(%v) to add refs %v, not %v", test.field, test.refs, refs)
		}
	}
}

func TestRenderMessagesAsDefinition(t *testing.T) {

	tests := []struct {
		descr    string
		msgDescs []*protodescriptor.DescriptorProto
		schema   map[string]swagger_options.Schema // per-message schema to add
		defs     swaggerDefinitionsObject
	}{
		{
			descr: "no swagger options",
			msgDescs: []*protodescriptor.DescriptorProto{
				&protodescriptor.DescriptorProto{Name: proto.String("Message")},
			},
			schema: map[string]swagger_options.Schema{},
			defs: map[string]swaggerSchemaObject{
				"Message": swaggerSchemaObject{schemaCore: schemaCore{Type: "object"}},
			},
		},
		{
			descr: "example option",
			msgDescs: []*protodescriptor.DescriptorProto{
				&protodescriptor.DescriptorProto{Name: proto.String("Message")},
			},
			schema: map[string]swagger_options.Schema{
				"Message": swagger_options.Schema{
					Example: &any.Any{
						TypeUrl: "this_isnt_used",
						Value:   []byte(`{"foo":"bar"}`),
					},
				},
			},
			defs: map[string]swaggerSchemaObject{
				"Message": swaggerSchemaObject{schemaCore: schemaCore{
					Type:    "object",
					Example: json.RawMessage(`{"foo":"bar"}`),
				}},
			},
		},
		{
			descr: "example option with something non-json",
			msgDescs: []*protodescriptor.DescriptorProto{
				&protodescriptor.DescriptorProto{Name: proto.String("Message")},
			},
			schema: map[string]swagger_options.Schema{
				"Message": swagger_options.Schema{
					Example: &any.Any{
						Value: []byte(`XXXX anything goes XXXX`),
					},
				},
			},
			defs: map[string]swaggerSchemaObject{
				"Message": swaggerSchemaObject{schemaCore: schemaCore{
					Type:    "object",
					Example: json.RawMessage(`XXXX anything goes XXXX`),
				}},
			},
		},
		{
			descr: "external docs option",
			msgDescs: []*protodescriptor.DescriptorProto{
				&protodescriptor.DescriptorProto{Name: proto.String("Message")},
			},
			schema: map[string]swagger_options.Schema{
				"Message": swagger_options.Schema{
					ExternalDocs: &swagger_options.ExternalDocumentation{
						Description: "glorious docs",
						Url:         "https://nada",
					},
				},
			},
			defs: map[string]swaggerSchemaObject{
				"Message": swaggerSchemaObject{
					schemaCore: schemaCore{
						Type: "object",
					},
					ExternalDocs: &swaggerExternalDocumentationObject{
						Description: "glorious docs",
						URL:         "https://nada",
					},
				},
			},
		},
		{
			descr: "JSONSchema options",
			msgDescs: []*protodescriptor.DescriptorProto{
				&protodescriptor.DescriptorProto{Name: proto.String("Message")},
			},
			schema: map[string]swagger_options.Schema{
				"Message": swagger_options.Schema{
					JsonSchema: &swagger_options.JSONSchema{
						Title:            "title",
						Description:      "desc",
						MultipleOf:       100,
						Maximum:          101,
						ExclusiveMaximum: true,
						Minimum:          1,
						ExclusiveMinimum: true,
						MaxLength:        10,
						MinLength:        3,
						Pattern:          "[a-z]+",
						MaxItems:         20,
						MinItems:         2,
						UniqueItems:      true,
						MaxProperties:    33,
						MinProperties:    22,
						Required:         []string{"req"},
						ReadOnly:         true,
					},
				},
			},
			defs: map[string]swaggerSchemaObject{
				"Message": swaggerSchemaObject{
					schemaCore: schemaCore{
						Type: "object",
					},
					Title:            "title",
					Description:      "desc",
					MultipleOf:       100,
					Maximum:          101,
					ExclusiveMaximum: true,
					Minimum:          1,
					ExclusiveMinimum: true,
					MaxLength:        10,
					MinLength:        3,
					Pattern:          "[a-z]+",
					MaxItems:         20,
					MinItems:         2,
					UniqueItems:      true,
					MaxProperties:    33,
					MinProperties:    22,
					Required:         []string{"req"},
					ReadOnly:         true,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.descr, func(t *testing.T) {

			msgs := []*descriptor.Message{}
			for _, msgdesc := range test.msgDescs {
				msgdesc.Options = &protodescriptor.MessageOptions{}
				msgs = append(msgs, &descriptor.Message{DescriptorProto: msgdesc})
			}

			reg := descriptor.NewRegistry()
			file := descriptor.File{
				FileDescriptorProto: &protodescriptor.FileDescriptorProto{
					SourceCodeInfo: &protodescriptor.SourceCodeInfo{},
					Name:           proto.String("example.proto"),
					Package:        proto.String("example"),
					Dependency:     []string{},
					MessageType:    test.msgDescs,
					EnumType:       []*protodescriptor.EnumDescriptorProto{},
					Service:        []*protodescriptor.ServiceDescriptorProto{},
				},
				Messages: msgs,
			}
			reg.Load(&plugin.CodeGeneratorRequest{
				ProtoFile: []*protodescriptor.FileDescriptorProto{file.FileDescriptorProto},
			})

			msgMap := map[string]*descriptor.Message{}
			for _, d := range test.msgDescs {
				name := d.GetName()
				msg, err := reg.LookupMsg("example", name)
				if err != nil {
					t.Fatalf("lookup message %v: %v", name, err)
				}
				msgMap[msg.FQMN()] = msg

				if schema, ok := test.schema[name]; ok {
					err := proto.SetExtension(d.Options, swagger_options.E_Openapiv2Schema, &schema)
					if err != nil {
						t.Fatalf("SetExtension(%s, ...) returned error: %v", msg, err)
					}
				}
			}

			refs := make(refMap)
			actual := make(swaggerDefinitionsObject)
			renderMessagesAsDefinition(msgMap, actual, reg, refs)

			if !reflect.DeepEqual(actual, test.defs) {
				t.Errorf("Expected renderMessagesAsDefinition() to add defs %+v, not %+v", test.defs, actual)
			}
		})
	}
}

func TestUpdateSwaggerDataFromComments(t *testing.T) {

	tests := []struct {
		descr                 string
		swaggerObject         interface{}
		comments              string
		expectedError         error
		expectedSwaggerObject interface{}
	}{
		{
			descr:                 "empty comments",
			swaggerObject:         nil,
			expectedSwaggerObject: nil,
			comments:              "",
			expectedError:         nil,
		},
		{
			descr:         "set field to read only",
			swaggerObject: &swaggerSchemaObject{},
			expectedSwaggerObject: &swaggerSchemaObject{
				ReadOnly:    true,
				Description: "... Output only. ...",
			},
			comments:      "... Output only. ...",
			expectedError: nil,
		},
		{
			descr:         "set title",
			swaggerObject: &swaggerSchemaObject{},
			expectedSwaggerObject: &swaggerSchemaObject{
				Title: "Comment with no trailing dot",
			},
			comments:      "Comment with no trailing dot",
			expectedError: nil,
		},
		{
			descr:         "set description",
			swaggerObject: &swaggerSchemaObject{},
			expectedSwaggerObject: &swaggerSchemaObject{
				Description: "Comment with trailing dot.",
			},
			comments:      "Comment with trailing dot.",
			expectedError: nil,
		},
		{
			descr: "use info object",
			swaggerObject: &swaggerObject{
				Info: swaggerInfoObject{},
			},
			expectedSwaggerObject: &swaggerObject{
				Info: swaggerInfoObject{
					Description: "Comment with trailing dot.",
				},
			},
			comments:      "Comment with trailing dot.",
			expectedError: nil,
		},
		{
			descr:         "multi line comment with title",
			swaggerObject: &swaggerSchemaObject{},
			expectedSwaggerObject: &swaggerSchemaObject{
				Title:       "First line",
				Description: "Second line",
			},
			comments:      "First line\n\nSecond line",
			expectedError: nil,
		},
		{
			descr:         "multi line comment no title",
			swaggerObject: &swaggerSchemaObject{},
			expectedSwaggerObject: &swaggerSchemaObject{
				Description: "First line.\n\nSecond line",
			},
			comments:      "First line.\n\nSecond line",
			expectedError: nil,
		},
		{
			descr:         "multi line comment with summary with dot",
			swaggerObject: &swaggerOperationObject{},
			expectedSwaggerObject: &swaggerOperationObject{
				Summary:     "First line.",
				Description: "Second line",
			},
			comments:      "First line.\n\nSecond line",
			expectedError: nil,
		},
		{
			descr:         "multi line comment with summary no dot",
			swaggerObject: &swaggerOperationObject{},
			expectedSwaggerObject: &swaggerOperationObject{
				Summary:     "First line",
				Description: "Second line",
			},
			comments:      "First line\n\nSecond line",
			expectedError: nil,
		},
		{
			descr:                 "multi line comment with summary no dot",
			swaggerObject:         &schemaCore{},
			expectedSwaggerObject: &schemaCore{},
			comments:              "Any comment",
			expectedError:         errors.New("no description nor summary property"),
		},
	}

	for _, test := range tests {
		t.Run(test.descr, func(t *testing.T) {
			err := updateSwaggerDataFromComments(test.swaggerObject, test.comments, false)

			if test.expectedError == nil {
				if err != nil {
					t.Errorf("unexpected error '%v'", err)
				}
				if !reflect.DeepEqual(test.swaggerObject, test.expectedSwaggerObject) {
					t.Errorf("swaggerObject was not updated corretly, expected '%+v', got '%+v'", test.expectedSwaggerObject, test.swaggerObject)
				}
			} else {
				if err == nil {
					t.Error("expected update error not returned")
				}
				if !reflect.DeepEqual(test.swaggerObject, test.expectedSwaggerObject) {
					t.Errorf("swaggerObject was not updated corretly, expected '%+v', got '%+v'", test.expectedSwaggerObject, test.swaggerObject)
				}
				if err.Error() != test.expectedError.Error() {
					t.Errorf("expected error malformed, expected %q, got %q", test.expectedError.Error(), err.Error())
				}
			}
		})
	}
}
