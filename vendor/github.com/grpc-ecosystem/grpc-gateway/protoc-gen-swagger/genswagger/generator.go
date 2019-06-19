package genswagger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	pbdescriptor "github.com/golang/protobuf/descriptor"
	"github.com/golang/protobuf/proto"
	protocdescriptor "github.com/golang/protobuf/protoc-gen-go/descriptor"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/grpc-ecosystem/grpc-gateway/internal"
	"github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway/descriptor"
	gen "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway/generator"
	swagger_options "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger/options"
)

var (
	errNoTargetService = errors.New("no target service defined in the file")
)

type generator struct {
	reg *descriptor.Registry
}

type wrapper struct {
	fileName string
	swagger  *swaggerObject
}

// New returns a new generator which generates grpc gateway files.
func New(reg *descriptor.Registry) gen.Generator {
	return &generator{reg: reg}
}

// Merge a lot of swagger file (wrapper) to single one swagger file
func mergeTargetFile(targets []*wrapper, mergeFileName string) *wrapper {
	var mergedTarget *wrapper
	for _, f := range targets {
		if mergedTarget == nil {
			mergedTarget = &wrapper{
				fileName: mergeFileName,
				swagger:  f.swagger,
			}
		} else {
			for k, v := range f.swagger.Definitions {
				mergedTarget.swagger.Definitions[k] = v
			}
			for k, v := range f.swagger.StreamDefinitions {
				mergedTarget.swagger.StreamDefinitions[k] = v
			}
			for k, v := range f.swagger.Paths {
				mergedTarget.swagger.Paths[k] = v
			}
			for k, v := range f.swagger.SecurityDefinitions {
				mergedTarget.swagger.SecurityDefinitions[k] = v
			}
			mergedTarget.swagger.Security = append(mergedTarget.swagger.Security, f.swagger.Security...)
		}
	}
	return mergedTarget
}

// convert swagger file obj to plugin.CodeGeneratorResponse_File
func encodeSwagger(file *wrapper) (*plugin.CodeGeneratorResponse_File, error) {
	var formatted bytes.Buffer
	enc := json.NewEncoder(&formatted)
	enc.SetIndent("", "  ")
	if err := enc.Encode(*file.swagger); err != nil {
		return nil, err
	}
	name := file.fileName
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	output := fmt.Sprintf("%s.swagger.json", base)
	return &plugin.CodeGeneratorResponse_File{
		Name:    proto.String(output),
		Content: proto.String(formatted.String()),
	}, nil
}

func (g *generator) Generate(targets []*descriptor.File) ([]*plugin.CodeGeneratorResponse_File, error) {
	var files []*plugin.CodeGeneratorResponse_File
	if g.reg.IsAllowMerge() {
		var mergedTarget *descriptor.File
		// try to find proto leader
		for _, f := range targets {
			if proto.HasExtension(f.Options, swagger_options.E_Openapiv2Swagger) {
				mergedTarget = f
				break
			}
		}
		// merge protos to leader
		for _, f := range targets {
			if mergedTarget == nil {
				mergedTarget = f
			} else {
				mergedTarget.Enums = append(mergedTarget.Enums, f.Enums...)
				mergedTarget.Messages = append(mergedTarget.Messages, f.Messages...)
				mergedTarget.Services = append(mergedTarget.Services, f.Services...)
			}
		}

		targets = nil
		targets = append(targets, mergedTarget)
	}

	var swaggers []*wrapper
	for _, file := range targets {
		glog.V(1).Infof("Processing %s", file.GetName())
		swagger, err := applyTemplate(param{File: file, reg: g.reg})
		if err == errNoTargetService {
			glog.V(1).Infof("%s: %v", file.GetName(), err)
			continue
		}
		if err != nil {
			return nil, err
		}
		swaggers = append(swaggers, &wrapper{
			fileName: file.GetName(),
			swagger:  swagger,
		})
	}

	if g.reg.IsAllowMerge() {
		targetSwagger := mergeTargetFile(swaggers, g.reg.GetMergeFileName())
		f, err := encodeSwagger(targetSwagger)
		if err != nil {
			return nil, fmt.Errorf("failed to encode swagger for %s: %s", g.reg.GetMergeFileName(), err)
		}
		files = append(files, f)
		glog.V(1).Infof("New swagger file will emit")
	} else {
		for _, file := range swaggers {
			f, err := encodeSwagger(file)
			if err != nil {
				return nil, fmt.Errorf("failed to encode swagger for %s: %s", file.fileName, err)
			}
			files = append(files, f)
			glog.V(1).Infof("New swagger file will emit")
		}
	}
	return files, nil
}

//AddStreamError Adds grpc.gateway.runtime.StreamError and google.protobuf.Any to registry for stream responses
func AddStreamError(reg *descriptor.Registry) error {
	//load internal protos
	any := fileDescriptorProtoForMessage(&any.Any{})
	streamError := fileDescriptorProtoForMessage(&internal.StreamError{})
	if err := reg.Load(&plugin.CodeGeneratorRequest{
		ProtoFile: []*protocdescriptor.FileDescriptorProto{
			any,
			streamError,
		},
	}); err != nil {
		return err
	}
	return nil
}

func fileDescriptorProtoForMessage(msg pbdescriptor.Message) *protocdescriptor.FileDescriptorProto {
	fdp, _ := pbdescriptor.ForMessage(msg)
	fdp.SourceCodeInfo = &protocdescriptor.SourceCodeInfo{}
	return fdp
}
