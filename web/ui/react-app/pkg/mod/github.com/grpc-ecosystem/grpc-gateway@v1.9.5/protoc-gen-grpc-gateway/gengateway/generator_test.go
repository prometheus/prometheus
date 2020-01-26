package gengateway

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	protodescriptor "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway/descriptor"
)

func newExampleFileDescriptor() *descriptor.File {
	return newExampleFileDescriptorWithGoPkg(
		&descriptor.GoPackage{
			Path: "example.com/path/to/example/example.pb",
			Name: "example_pb",
		},
	)
}

func newExampleFileDescriptorWithGoPkg(gp *descriptor.GoPackage) *descriptor.File {
	msgdesc := &protodescriptor.DescriptorProto{
		Name: proto.String("ExampleMessage"),
	}
	msg := &descriptor.Message{
		DescriptorProto: msgdesc,
	}
	msg1 := &descriptor.Message{
		DescriptorProto: msgdesc,
		File: &descriptor.File{
			GoPkg: descriptor.GoPackage{
				Path: "github.com/golang/protobuf/ptypes/empty",
				Name: "empty",
			},
		},
	}
	meth := &protodescriptor.MethodDescriptorProto{
		Name:       proto.String("Example"),
		InputType:  proto.String("ExampleMessage"),
		OutputType: proto.String("ExampleMessage"),
	}
	meth1 := &protodescriptor.MethodDescriptorProto{
		Name:       proto.String("ExampleWithoutBindings"),
		InputType:  proto.String("empty.Empty"),
		OutputType: proto.String("empty.Empty"),
	}
	svc := &protodescriptor.ServiceDescriptorProto{
		Name:   proto.String("ExampleService"),
		Method: []*protodescriptor.MethodDescriptorProto{meth, meth1},
	}
	return &descriptor.File{
		FileDescriptorProto: &protodescriptor.FileDescriptorProto{
			Name:        proto.String("example.proto"),
			Package:     proto.String("example"),
			Dependency:  []string{"a.example/b/c.proto", "a.example/d/e.proto"},
			MessageType: []*protodescriptor.DescriptorProto{msgdesc},
			Service:     []*protodescriptor.ServiceDescriptorProto{svc},
		},
		GoPkg:    *gp,
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
							},
						},
					},
					{
						MethodDescriptorProto: meth1,
						RequestType:           msg1,
						ResponseType:          msg1,
					},
				},
			},
		},
	}
}

func TestGenerateServiceWithoutBindings(t *testing.T) {
	file := newExampleFileDescriptor()
	g := &generator{}
	got, err := g.generate(crossLinkFixture(file))
	if err != nil {
		t.Errorf("generate(%#v) failed with %v; want success", file, err)
		return
	}
	if notwanted := `"github.com/golang/protobuf/ptypes/empty"`; strings.Contains(got, notwanted) {
		t.Errorf("generate(%#v) = %s; does not want to contain %s", file, got, notwanted)
	}
}

func TestGenerateOutputPath(t *testing.T) {
	cases := []struct {
		file     *descriptor.File
		pathType pathType
		expected string
	}{
		{
			file: newExampleFileDescriptorWithGoPkg(
				&descriptor.GoPackage{
					Path: "example.com/path/to/example",
					Name: "example_pb",
				},
			),
			expected: "example.com/path/to/example",
		},
		{
			file: newExampleFileDescriptorWithGoPkg(
				&descriptor.GoPackage{
					Path: "example",
					Name: "example_pb",
				},
			),
			expected: "example",
		},
		{
			file: newExampleFileDescriptorWithGoPkg(
				&descriptor.GoPackage{
					Path: "example.com/path/to/example",
					Name: "example_pb",
				},
			),
			pathType: pathTypeSourceRelative,
			expected: ".",
		},
		{
			file: newExampleFileDescriptorWithGoPkg(
				&descriptor.GoPackage{
					Path: "example",
					Name: "example_pb",
				},
			),
			pathType: pathTypeSourceRelative,
			expected: ".",
		},
	}

	for _, c := range cases {
		g := &generator{pathType: c.pathType}

		file := c.file
		gots, err := g.Generate([]*descriptor.File{crossLinkFixture(file)})
		if err != nil {
			t.Errorf("Generate(%#v) failed with %v; wants success", file, err)
			return
		}

		if len(gots) != 1 {
			t.Errorf("Generate(%#v) failed; expects on result got %d", file, len(gots))
			return
		}

		got := gots[0]
		if got.Name == nil {
			t.Errorf("Generate(%#v) failed; expects non-nil Name(%v)", file, got.Name)
			return
		}

		gotPath := filepath.Dir(*got.Name)
		expectedPath := c.expected
		if gotPath != expectedPath {
			t.Errorf("Generate(%#v) failed; got path: %s expected path: %s", file, gotPath, expectedPath)
			return
		}
	}
}
