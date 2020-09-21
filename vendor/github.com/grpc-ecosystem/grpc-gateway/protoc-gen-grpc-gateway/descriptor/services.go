package descriptor

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	descriptor "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway/httprule"
	options "google.golang.org/genproto/googleapis/api/annotations"
)

// loadServices registers services and their methods from "targetFile" to "r".
// It must be called after loadFile is called for all files so that loadServices
// can resolve names of message types and their fields.
func (r *Registry) loadServices(file *File) error {
	glog.V(1).Infof("Loading services from %s", file.GetName())
	var svcs []*Service
	for _, sd := range file.GetService() {
		glog.V(2).Infof("Registering %s", sd.GetName())
		svc := &Service{
			File:                   file,
			ServiceDescriptorProto: sd,
		}
		for _, md := range sd.GetMethod() {
			glog.V(2).Infof("Processing %s.%s", sd.GetName(), md.GetName())
			opts, err := extractAPIOptions(md)
			if err != nil {
				glog.Errorf("Failed to extract HttpRule from %s.%s: %v", svc.GetName(), md.GetName(), err)
				return err
			}
			optsList := r.LookupExternalHTTPRules((&Method{Service: svc, MethodDescriptorProto: md}).FQMN())
			if opts != nil {
				optsList = append(optsList, opts)
			}
			if len(optsList) == 0 {
				glog.Warningf("No HttpRule found for method: %s.%s", svc.GetName(), md.GetName())
			}
			meth, err := r.newMethod(svc, md, optsList)
			if err != nil {
				return err
			}
			svc.Methods = append(svc.Methods, meth)
		}
		if len(svc.Methods) == 0 {
			continue
		}
		glog.V(2).Infof("Registered %s with %d method(s)", svc.GetName(), len(svc.Methods))
		svcs = append(svcs, svc)
	}
	file.Services = svcs
	return nil
}

func (r *Registry) newMethod(svc *Service, md *descriptor.MethodDescriptorProto, optsList []*options.HttpRule) (*Method, error) {
	requestType, err := r.LookupMsg(svc.File.GetPackage(), md.GetInputType())
	if err != nil {
		return nil, err
	}
	responseType, err := r.LookupMsg(svc.File.GetPackage(), md.GetOutputType())
	if err != nil {
		return nil, err
	}
	meth := &Method{
		Service:               svc,
		MethodDescriptorProto: md,
		RequestType:           requestType,
		ResponseType:          responseType,
	}

	newBinding := func(opts *options.HttpRule, idx int) (*Binding, error) {
		var (
			httpMethod   string
			pathTemplate string
		)
		switch {
		case opts.GetGet() != "":
			httpMethod = "GET"
			pathTemplate = opts.GetGet()
			if opts.Body != "" {
				return nil, fmt.Errorf("must not set request body when http method is GET: %s", md.GetName())
			}

		case opts.GetPut() != "":
			httpMethod = "PUT"
			pathTemplate = opts.GetPut()

		case opts.GetPost() != "":
			httpMethod = "POST"
			pathTemplate = opts.GetPost()

		case opts.GetDelete() != "":
			httpMethod = "DELETE"
			pathTemplate = opts.GetDelete()
			if opts.Body != "" && !r.allowDeleteBody {
				return nil, fmt.Errorf("must not set request body when http method is DELETE except allow_delete_body option is true: %s", md.GetName())
			}

		case opts.GetPatch() != "":
			httpMethod = "PATCH"
			pathTemplate = opts.GetPatch()

		case opts.GetCustom() != nil:
			custom := opts.GetCustom()
			httpMethod = custom.Kind
			pathTemplate = custom.Path

		default:
			glog.V(1).Infof("No pattern specified in google.api.HttpRule: %s", md.GetName())
			return nil, nil
		}

		parsed, err := httprule.Parse(pathTemplate)
		if err != nil {
			return nil, err
		}
		tmpl := parsed.Compile()

		if md.GetClientStreaming() && len(tmpl.Fields) > 0 {
			return nil, fmt.Errorf("cannot use path parameter in client streaming")
		}

		b := &Binding{
			Method:     meth,
			Index:      idx,
			PathTmpl:   tmpl,
			HTTPMethod: httpMethod,
		}

		for _, f := range tmpl.Fields {
			param, err := r.newParam(meth, f)
			if err != nil {
				return nil, err
			}
			b.PathParams = append(b.PathParams, param)
		}

		// TODO(yugui) Handle query params

		b.Body, err = r.newBody(meth, opts.Body)
		if err != nil {
			return nil, err
		}

		b.ResponseBody, err = r.newResponse(meth, opts.ResponseBody)
		if err != nil {
			return nil, err
		}

		return b, nil
	}

	applyOpts := func(opts *options.HttpRule) error {
		b, err := newBinding(opts, len(meth.Bindings))
		if err != nil {
			return err
		}

		if b != nil {
			meth.Bindings = append(meth.Bindings, b)
		}
		for _, additional := range opts.GetAdditionalBindings() {
			if len(additional.AdditionalBindings) > 0 {
				return fmt.Errorf("additional_binding in additional_binding not allowed: %s.%s", svc.GetName(), meth.GetName())
			}
			b, err := newBinding(additional, len(meth.Bindings))
			if err != nil {
				return err
			}
			meth.Bindings = append(meth.Bindings, b)
		}

		return nil
	}

	for _, opts := range optsList {
		if err := applyOpts(opts); err != nil {
			return nil, err
		}
	}

	return meth, nil
}

func extractAPIOptions(meth *descriptor.MethodDescriptorProto) (*options.HttpRule, error) {
	if meth.Options == nil {
		return nil, nil
	}
	if !proto.HasExtension(meth.Options, options.E_Http) {
		return nil, nil
	}
	ext, err := proto.GetExtension(meth.Options, options.E_Http)
	if err != nil {
		return nil, err
	}
	opts, ok := ext.(*options.HttpRule)
	if !ok {
		return nil, fmt.Errorf("extension is %T; want an HttpRule", ext)
	}
	return opts, nil
}

func (r *Registry) newParam(meth *Method, path string) (Parameter, error) {
	msg := meth.RequestType
	fields, err := r.resolveFieldPath(msg, path, true)
	if err != nil {
		return Parameter{}, err
	}
	l := len(fields)
	if l == 0 {
		return Parameter{}, fmt.Errorf("invalid field access list for %s", path)
	}
	target := fields[l-1].Target
	switch target.GetType() {
	case descriptor.FieldDescriptorProto_TYPE_MESSAGE, descriptor.FieldDescriptorProto_TYPE_GROUP:
		glog.V(2).Infoln("found aggregate type:", target, target.TypeName)
		if IsWellKnownType(*target.TypeName) {
			glog.V(2).Infoln("found well known aggregate type:", target)
		} else {
			return Parameter{}, fmt.Errorf("aggregate type %s in parameter of %s.%s: %s", target.Type, meth.Service.GetName(), meth.GetName(), path)
		}
	}
	return Parameter{
		FieldPath: FieldPath(fields),
		Method:    meth,
		Target:    fields[l-1].Target,
	}, nil
}

func (r *Registry) newBody(meth *Method, path string) (*Body, error) {
	msg := meth.RequestType
	switch path {
	case "":
		return nil, nil
	case "*":
		return &Body{FieldPath: nil}, nil
	}
	fields, err := r.resolveFieldPath(msg, path, false)
	if err != nil {
		return nil, err
	}
	return &Body{FieldPath: FieldPath(fields)}, nil
}

func (r *Registry) newResponse(meth *Method, path string) (*Body, error) {
	msg := meth.ResponseType
	switch path {
	case "", "*":
		return nil, nil
	}
	fields, err := r.resolveFieldPath(msg, path, false)
	if err != nil {
		return nil, err
	}
	return &Body{FieldPath: FieldPath(fields)}, nil
}

// lookupField looks up a field named "name" within "msg".
// It returns nil if no such field found.
func lookupField(msg *Message, name string) *Field {
	for _, f := range msg.Fields {
		if f.GetName() == name {
			return f
		}
	}
	return nil
}

// resolveFieldPath resolves "path" into a list of fieldDescriptor, starting from "msg".
func (r *Registry) resolveFieldPath(msg *Message, path string, isPathParam bool) ([]FieldPathComponent, error) {
	if path == "" {
		return nil, nil
	}

	root := msg
	var result []FieldPathComponent
	for i, c := range strings.Split(path, ".") {
		if i > 0 {
			f := result[i-1].Target
			switch f.GetType() {
			case descriptor.FieldDescriptorProto_TYPE_MESSAGE, descriptor.FieldDescriptorProto_TYPE_GROUP:
				var err error
				msg, err = r.LookupMsg(msg.FQMN(), f.GetTypeName())
				if err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("not an aggregate type: %s in %s", f.GetName(), path)
			}
		}

		glog.V(2).Infof("Lookup %s in %s", c, msg.FQMN())
		f := lookupField(msg, c)
		if f == nil {
			return nil, fmt.Errorf("no field %q found in %s", path, root.GetName())
		}
		if !(isPathParam || r.allowRepeatedFieldsInBody) && f.GetLabel() == descriptor.FieldDescriptorProto_LABEL_REPEATED {
			return nil, fmt.Errorf("repeated field not allowed in field path: %s in %s", f.GetName(), path)
		}
		result = append(result, FieldPathComponent{Name: c, Target: f})
	}
	return result, nil
}
