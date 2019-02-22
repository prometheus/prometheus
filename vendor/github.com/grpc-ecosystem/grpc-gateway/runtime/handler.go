package runtime

import (
	"fmt"
	"io"
	"net/http"
	"net/textproto"

	"context"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/grpc-ecosystem/grpc-gateway/internal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/status"
)

// ForwardResponseStream forwards the stream from gRPC server to REST client.
func ForwardResponseStream(ctx context.Context, mux *ServeMux, marshaler Marshaler, w http.ResponseWriter, req *http.Request, recv func() (proto.Message, error), opts ...func(context.Context, http.ResponseWriter, proto.Message) error) {
	f, ok := w.(http.Flusher)
	if !ok {
		grpclog.Infof("Flush not supported in %T", w)
		http.Error(w, "unexpected type of web server", http.StatusInternalServerError)
		return
	}

	md, ok := ServerMetadataFromContext(ctx)
	if !ok {
		grpclog.Infof("Failed to extract ServerMetadata from context")
		http.Error(w, "unexpected error", http.StatusInternalServerError)
		return
	}
	handleForwardResponseServerMetadata(w, mux, md)

	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Content-Type", marshaler.ContentType())
	if err := handleForwardResponseOptions(ctx, w, nil, opts); err != nil {
		HTTPError(ctx, mux, marshaler, w, req, err)
		return
	}

	var delimiter []byte
	if d, ok := marshaler.(Delimited); ok {
		delimiter = d.Delimiter()
	} else {
		delimiter = []byte("\n")
	}

	var wroteHeader bool
	for {
		resp, err := recv()
		if err == io.EOF {
			return
		}
		if err != nil {
			handleForwardResponseStreamError(wroteHeader, marshaler, w, err)
			return
		}
		if err := handleForwardResponseOptions(ctx, w, resp, opts); err != nil {
			handleForwardResponseStreamError(wroteHeader, marshaler, w, err)
			return
		}

		buf, err := marshaler.Marshal(streamChunk(resp, nil))
		if err != nil {
			grpclog.Infof("Failed to marshal response chunk: %v", err)
			handleForwardResponseStreamError(wroteHeader, marshaler, w, err)
			return
		}
		if _, err = w.Write(buf); err != nil {
			grpclog.Infof("Failed to send response chunk: %v", err)
			return
		}
		wroteHeader = true
		if _, err = w.Write(delimiter); err != nil {
			grpclog.Infof("Failed to send delimiter chunk: %v", err)
			return
		}
		f.Flush()
	}
}

func handleForwardResponseServerMetadata(w http.ResponseWriter, mux *ServeMux, md ServerMetadata) {
	for k, vs := range md.HeaderMD {
		if h, ok := mux.outgoingHeaderMatcher(k); ok {
			for _, v := range vs {
				w.Header().Add(h, v)
			}
		}
	}
}

func handleForwardResponseTrailerHeader(w http.ResponseWriter, md ServerMetadata) {
	for k := range md.TrailerMD {
		tKey := textproto.CanonicalMIMEHeaderKey(fmt.Sprintf("%s%s", MetadataTrailerPrefix, k))
		w.Header().Add("Trailer", tKey)
	}
}

func handleForwardResponseTrailer(w http.ResponseWriter, md ServerMetadata) {
	for k, vs := range md.TrailerMD {
		tKey := fmt.Sprintf("%s%s", MetadataTrailerPrefix, k)
		for _, v := range vs {
			w.Header().Add(tKey, v)
		}
	}
}

// responseBody interface contains method for getting field for marshaling to the response body
// this method is generated for response struct from the value of `response_body` in the `google.api.HttpRule`
type responseBody interface {
	XXX_ResponseBody() interface{}
}

// ForwardResponseMessage forwards the message "resp" from gRPC server to REST client.
func ForwardResponseMessage(ctx context.Context, mux *ServeMux, marshaler Marshaler, w http.ResponseWriter, req *http.Request, resp proto.Message, opts ...func(context.Context, http.ResponseWriter, proto.Message) error) {
	md, ok := ServerMetadataFromContext(ctx)
	if !ok {
		grpclog.Infof("Failed to extract ServerMetadata from context")
	}

	handleForwardResponseServerMetadata(w, mux, md)
	handleForwardResponseTrailerHeader(w, md)

	contentType := marshaler.ContentType()
	// Check marshaler on run time in order to keep backwards compatability
	// An interface param needs to be added to the ContentType() function on 
	// the Marshal interface to be able to remove this check
	if httpBodyMarshaler, ok := marshaler.(*HTTPBodyMarshaler); ok {
		contentType = httpBodyMarshaler.ContentTypeFromMessage(resp)
	}
	w.Header().Set("Content-Type", contentType)

	if err := handleForwardResponseOptions(ctx, w, resp, opts); err != nil {
		HTTPError(ctx, mux, marshaler, w, req, err)
		return
	}
	var buf []byte
	var err error
	if rb, ok := resp.(responseBody); ok {
		buf, err = marshaler.Marshal(rb.XXX_ResponseBody())
	} else {
		buf, err = marshaler.Marshal(resp)
	}
	if err != nil {
		grpclog.Infof("Marshal error: %v", err)
		HTTPError(ctx, mux, marshaler, w, req, err)
		return
	}

	if _, err = w.Write(buf); err != nil {
		grpclog.Infof("Failed to write response: %v", err)
	}

	handleForwardResponseTrailer(w, md)
}

func handleForwardResponseOptions(ctx context.Context, w http.ResponseWriter, resp proto.Message, opts []func(context.Context, http.ResponseWriter, proto.Message) error) error {
	if len(opts) == 0 {
		return nil
	}
	for _, opt := range opts {
		if err := opt(ctx, w, resp); err != nil {
			grpclog.Infof("Error handling ForwardResponseOptions: %v", err)
			return err
		}
	}
	return nil
}

func handleForwardResponseStreamError(wroteHeader bool, marshaler Marshaler, w http.ResponseWriter, err error) {
	buf, merr := marshaler.Marshal(streamChunk(nil, err))
	if merr != nil {
		grpclog.Infof("Failed to marshal an error: %v", merr)
		return
	}
	if !wroteHeader {
		s, ok := status.FromError(err)
		if !ok {
			s = status.New(codes.Unknown, err.Error())
		}
		w.WriteHeader(HTTPStatusFromCode(s.Code()))
	}
	if _, werr := w.Write(buf); werr != nil {
		grpclog.Infof("Failed to notify error to client: %v", werr)
		return
	}
}

func streamChunk(result proto.Message, err error) map[string]proto.Message {
	if err != nil {
		grpcCode := codes.Unknown
		grpcMessage := err.Error()
		var grpcDetails []*any.Any
		if s, ok := status.FromError(err); ok {
			grpcCode = s.Code()
			grpcMessage = s.Message()
			grpcDetails = s.Proto().GetDetails()
		}
		httpCode := HTTPStatusFromCode(grpcCode)
		return map[string]proto.Message{
			"error": &internal.StreamError{
				GrpcCode:   int32(grpcCode),
				HttpCode:   int32(httpCode),
				Message:    grpcMessage,
				HttpStatus: http.StatusText(httpCode),
				Details:    grpcDetails,
			},
		}
	}
	if result == nil {
		return streamChunk(nil, fmt.Errorf("empty response"))
	}
	return map[string]proto.Message{"result": result}
}
