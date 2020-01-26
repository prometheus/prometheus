---
title: Customizing your gateway
category: documentation
order: 101
---

# Customizing your gateway

## Message serialization
### Custom serializer

You might want to serialize request/response messages in MessagePack instead of JSON, for example.

1. Write a custom implementation of [`Marshaler`](http://godoc.org/github.com/grpc-ecosystem/grpc-gateway/runtime#Marshaler)
2. Register your marshaler with [`WithMarshalerOption`](http://godoc.org/github.com/grpc-ecosystem/grpc-gateway/runtime#WithMarshalerOption)
   e.g.
   ```go
   var m your.MsgPackMarshaler
   mux := runtime.NewServeMux(runtime.WithMarshalerOption("application/x-msgpack", m))
   ```

You can see [the default implementation for JSON](https://github.com/grpc-ecosystem/grpc-gateway/blob/master/runtime/marshal_jsonpb.go) for reference.

### Using camelCase for JSON

The protocol buffer compiler generates camelCase JSON tags that can be used with jsonpb package. By default jsonpb Marshaller uses `OrigName: true` which uses the exact case used in the proto files. To use camelCase for the JSON representation,
   ```go
   mux := runtime.NewServeMux(runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{OrigName:false}))
   ```

### Pretty-print JSON responses when queried with ?pretty

You can have Elasticsearch-style `?pretty` support in your gateway's endpoints as follows:

1. Wrap the ServeMux using a stdlib [`http.HandlerFunc`](https://golang.org/pkg/net/http/#HandlerFunc)
   that translates the provided query parameter into a custom `Accept` header, and
2. Register a pretty-printing marshaler for that MIME code.

For example:

```go
mux := runtime.NewServeMux(
	runtime.WithMarshalerOption("application/json+pretty", &runtime.JSONPb{Indent: "  "}),
)
prettier := func(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// checking Values as map[string][]string also catches ?pretty and ?pretty=
		// r.URL.Query().Get("pretty") would not.
		if _, ok := r.URL.Query()["pretty"]; ok {
			r.Header.Set("Accept", "application/json+pretty")
		}
		h.ServeHTTP(w, r)
	})
}
http.ListenAndServe(":8080", prettier(mux))
```

Note that  `runtime.JSONPb{Indent: "  "}` will do the trick for pretty-printing: it wraps
`jsonpb.Marshaler`:
```go
type Marshaler struct {
	// ...

	// A string to indent each level by. The presence of this field will
	// also cause a space to appear between the field separator and
	// value, and for newlines to be appear between fields and array
	// elements.
	Indent string

	// ...
}
```

Now, either when passing the header `Accept: application/json+pretty` or appending `?pretty` to
your HTTP endpoints, the response will be pretty-printed.

Note that this will conflict with any methods having input messages with fields named `pretty`;
also, this example code does not remove the query parameter `pretty` from further processing.

## Mapping from HTTP request headers to gRPC client metadata
You might not like [the default mapping rule](http://godoc.org/github.com/grpc-ecosystem/grpc-gateway/runtime#DefaultHeaderMatcher) and might want to pass through all the HTTP headers, for example.

1. Write a [`HeaderMatcherFunc`](http://godoc.org/github.com/grpc-ecosystem/grpc-gateway/runtime#HeaderMatcherFunc).
2. Register the function with [`WithIncomingHeaderMatcher`](http://godoc.org/github.com/grpc-ecosystem/grpc-gateway/runtime#WithIncomingHeaderMatcher)

   e.g.
   ```go
   func yourMatcher(headerName string) (mdName string, ok bool) {
   	...
   }
   ...
   mux := runtime.NewServeMux(runtime.WithIncomingHeaderMatcher(yourMatcher))

   ```

## Mapping from gRPC server metadata to HTTP response headers
ditto. Use [`WithOutgoingHeaderMatcher`](http://godoc.org/github.com/grpc-ecosystem/grpc-gateway/runtime#WithOutgoingHeaderMatcher)

## Mutate response messages or set response headers
You might want to return a subset of response fields as HTTP response headers; 
You might want to simply set an application-specific token in a header.
Or you might want to mutate the response messages to be returned.

1. Write a filter function.
   ```go
   func myFilter(ctx context.Context, w http.ResponseWriter, resp proto.Message) error {
   	w.Header().Set("X-My-Tracking-Token", resp.Token)
   	resp.Token = ""
   	return nil
   }
   ```
2. Register the filter with [`WithForwardResponseOption`](http://godoc.org/github.com/grpc-ecosystem/grpc-gateway/runtime#WithForwardResponseOption)
   
   e.g.
   ```go
   mux := runtime.NewServeMux(runtime.WithForwardResponseOption(myFilter))
   ```

## OpenTracing Support

If your project uses [OpenTracing](https://github.com/opentracing/opentracing-go) and you'd like spans to propagate through the gateway, you can add some middleware which parses the incoming HTTP headers to create a new span correctly.

```go
import (
   ...
   "github.com/opentracing/opentracing-go"
   "github.com/opentracing/opentracing-go/ext"
)

var grpcGatewayTag = opentracing.Tag{Key: string(ext.Component), Value: "grpc-gateway"}

func tracingWrapper(h http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    parentSpanContext, err := opentracing.GlobalTracer().Extract(
      opentracing.HTTPHeaders,
      opentracing.HTTPHeadersCarrier(r.Header))
    if err == nil || err == opentracing.ErrSpanContextNotFound {
      serverSpan := opentracing.GlobalTracer().StartSpan(
        "ServeHTTP",
        // this is magical, it attaches the new span to the parent parentSpanContext, and creates an unparented one if empty.
        ext.RPCServerOption(parentSpanContext),
        grpcGatewayTag,
      )
      r = r.WithContext(opentracing.ContextWithSpan(r.Context(), serverSpan))
      defer serverSpan.Finish()
    }
    h.ServeHTTP(w, r)
  })
}

// Then just wrap the mux returned by runtime.NewServeMux() like this
if err := http.ListenAndServe(":8080", tracingWrapper(mux)); err != nil {
  log.Fatalf("failed to start gateway server on 8080: %v", err)
}
```

## Error handler
http://mycodesmells.com/post/grpc-gateway-error-handler

## Stream Error Handler
The error handler described in the previous section applies only
to RPC methods that have a unary response.

When the method has a streaming response, grpc-gateway handles
that by emitting a newline-separated stream of "chunks". Each
chunk is an envelope that can container either a response message
or an error. Only the last chunk will include an error, and only
when the RPC handler ends abnormally (i.e. with an error code).

Because of the way the errors are included in the response body,
the other error handler signature is insufficient. So for server
streams, you must install a _different_ error handler:

```go
mux := runtime.NewServeMux(
	runtime.WithStreamErrorHandler(handleStreamError))
```

The signature of the handler is much more rigid because we need
to know the structure of the error payload in order to properly
encode the "chunk" schema into a Swagger/OpenAPI spec.

So the function must return a `*runtime.StreamError`. The handler
can choose to omit some fields and can filter/transform the original
error, such as stripping stack traces from error messages.

Here's an example custom handler:
```go
// handleStreamError overrides default behavior for computing an error
// message for a server stream.
//
// It uses a default "502 Bad Gateway" HTTP code; only emits "safe"
// messages; and does not set gRPC code or details fields (so they will
// be omitted from the resulting JSON object that is sent to client).
func handleStreamError(ctx context.Context, err error) *runtime.StreamError {
	code := http.StatusBadGateway
	msg := "unexpected error"
	if s, ok := status.FromError(err); ok {
		code = runtime.HTTPStatusFromCode(s.Code())
		// default message, based on the name of the gRPC code
		msg = code.String()
		// see if error details include "safe" message to send
		// to external callers
		for _, msg := s.Details() {
			if safe, ok := msg.(*SafeMessage); ok {
				msg = safe.Text
				break
			}
		}
	}
	return &runtime.StreamError{
	    HttpCode:   int32(code),
	    HttpStatus: http.StatusText(code),
	    Message:    msg,
	}
}
```

If no custom handler is provided, the default stream error handler
will include any gRPC error attributes (code, message, detail messages),
if the error being reported includes them. If the error does not have
these attributes, a gRPC code of `Unknown` (2) is reported. The default
handler will also include an HTTP code and status, which is derived
from the gRPC code (or set to `"500 Internal Server Error"` when
the source error has no gRPC attributes).

## Replace a response forwarder per method
You might want to keep the behavior of the current marshaler but change only a message forwarding of a certain API method.

1. write a custom forwarder which is compatible to [`ForwardResponseMessage`](http://godoc.org/github.com/grpc-ecosystem/grpc-gateway/runtime#ForwardResponseMessage) or [`ForwardResponseStream`](http://godoc.org/github.com/grpc-ecosystem/grpc-gateway/runtime#ForwardResponseStream).
2. replace the default forwarder of the method with your one.

   e.g. add `forwarder_overwrite.go` into the go package of the generated code,
   ```go
   package generated
   
   import (
   	"net/http"

   	"github.com/grpc-ecosystem/grpc-gateway/runtime"
   	"github.com/golang/protobuf/proto"
   	"golang.org/x/net/context"
   )

   func forwardCheckoutResp(ctx context.Context, mux *runtime.ServeMux, marshaler runtime.Marshaler, w http.ResponseWriter, req *http.Request, resp proto.Message, opts ...func(context.Context, http.ResponseWriter, proto.Message) error) {
   	if someCondition(resp) {
   		http.Error(w, "not enough credit", http. StatusPaymentRequired)
   		return
   	}
   	runtime.ForwardResponseMessage(ctx, mux, marshaler, w, req, resp, opts...)
   }
   
   func init() {
   	forward_MyService_Checkout_0 = forwardCheckoutResp
   }
   ```
