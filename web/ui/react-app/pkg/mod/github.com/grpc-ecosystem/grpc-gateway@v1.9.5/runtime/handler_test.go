package runtime_test

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"context"
	"github.com/golang/protobuf/proto"
	pb "github.com/grpc-ecosystem/grpc-gateway/examples/proto/examplepb"
	"github.com/grpc-ecosystem/grpc-gateway/internal"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestForwardResponseStream(t *testing.T) {
	type msg struct {
		pb  proto.Message
		err error
	}
	tests := []struct {
		name       string
		msgs       []msg
		statusCode int
	}{{
		name: "encoding",
		msgs: []msg{
			{&pb.SimpleMessage{Id: "One"}, nil},
			{&pb.SimpleMessage{Id: "Two"}, nil},
		},
		statusCode: http.StatusOK,
	}, {
		name:       "empty",
		statusCode: http.StatusOK,
	}, {
		name:       "error",
		msgs:       []msg{{nil, grpc.Errorf(codes.OutOfRange, "400")}},
		statusCode: http.StatusBadRequest,
	}, {
		name: "stream_error",
		msgs: []msg{
			{&pb.SimpleMessage{Id: "One"}, nil},
			{nil, grpc.Errorf(codes.OutOfRange, "400")},
		},
		statusCode: http.StatusOK,
	}}

	newTestRecv := func(t *testing.T, msgs []msg) func() (proto.Message, error) {
		var count int
		return func() (proto.Message, error) {
			if count == len(msgs) {
				return nil, io.EOF
			} else if count > len(msgs) {
				t.Errorf("recv() called %d times for %d messages", count, len(msgs))
			}
			count++
			msg := msgs[count-1]
			return msg.pb, msg.err
		}
	}
	ctx := runtime.NewServerMetadataContext(context.Background(), runtime.ServerMetadata{})
	marshaler := &runtime.JSONPb{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recv := newTestRecv(t, tt.msgs)
			req := httptest.NewRequest("GET", "http://example.com/foo", nil)
			resp := httptest.NewRecorder()

			runtime.ForwardResponseStream(ctx, runtime.NewServeMux(), marshaler, resp, req, recv)

			w := resp.Result()
			if w.StatusCode != tt.statusCode {
				t.Errorf("StatusCode %d want %d", w.StatusCode, tt.statusCode)
			}
			if h := w.Header.Get("Transfer-Encoding"); h != "chunked" {
				t.Errorf("ForwardResponseStream missing header chunked")
			}
			body, err := ioutil.ReadAll(w.Body)
			if err != nil {
				t.Errorf("Failed to read response body with %v", err)
			}
			w.Body.Close()

			var want []byte
			for i, msg := range tt.msgs {
				if msg.err != nil {
					if i == 0 {
						// Skip non-stream errors
						t.Skip("checking error encodings")
					}
					st, _ := status.FromError(msg.err)
					httpCode := runtime.HTTPStatusFromCode(st.Code())
					b, err := marshaler.Marshal(map[string]proto.Message{
						"error": &internal.StreamError{
							GrpcCode:   int32(st.Code()),
							HttpCode:   int32(httpCode),
							Message:    st.Message(),
							HttpStatus: http.StatusText(httpCode),
							Details:    st.Proto().GetDetails(),
						},
					})
					if err != nil {
						t.Errorf("marshaler.Marshal() failed %v", err)
					}
					errBytes := body[len(want):]
					if string(errBytes) != string(b) {
						t.Errorf("ForwardResponseStream() = \"%s\" want \"%s\"", errBytes, b)
					}

					return
				}
				b, err := marshaler.Marshal(map[string]proto.Message{"result": msg.pb})
				if err != nil {
					t.Errorf("marshaler.Marshal() failed %v", err)
				}
				want = append(want, b...)
				want = append(want, marshaler.Delimiter()...)
			}

			if string(body) != string(want) {
				t.Errorf("ForwardResponseStream() = \"%s\" want \"%s\"", body, want)
			}
		})
	}
}

// A custom marshaler implementation, that doesn't implement the delimited interface
type CustomMarshaler struct {
	m *runtime.JSONPb
}

func (c *CustomMarshaler) Marshal(v interface{}) ([]byte, error)      { return c.m.Marshal(v) }
func (c *CustomMarshaler) Unmarshal(data []byte, v interface{}) error { return c.m.Unmarshal(data, v) }
func (c *CustomMarshaler) NewDecoder(r io.Reader) runtime.Decoder     { return c.m.NewDecoder(r) }
func (c *CustomMarshaler) NewEncoder(w io.Writer) runtime.Encoder     { return c.m.NewEncoder(w) }
func (c *CustomMarshaler) ContentType() string                        { return c.m.ContentType() }

func TestForwardResponseStreamCustomMarshaler(t *testing.T) {
	type msg struct {
		pb  proto.Message
		err error
	}
	tests := []struct {
		name       string
		msgs       []msg
		statusCode int
	}{{
		name: "encoding",
		msgs: []msg{
			{&pb.SimpleMessage{Id: "One"}, nil},
			{&pb.SimpleMessage{Id: "Two"}, nil},
		},
		statusCode: http.StatusOK,
	}, {
		name:       "empty",
		statusCode: http.StatusOK,
	}, {
		name:       "error",
		msgs:       []msg{{nil, grpc.Errorf(codes.OutOfRange, "400")}},
		statusCode: http.StatusBadRequest,
	}, {
		name: "stream_error",
		msgs: []msg{
			{&pb.SimpleMessage{Id: "One"}, nil},
			{nil, grpc.Errorf(codes.OutOfRange, "400")},
		},
		statusCode: http.StatusOK,
	}}

	newTestRecv := func(t *testing.T, msgs []msg) func() (proto.Message, error) {
		var count int
		return func() (proto.Message, error) {
			if count == len(msgs) {
				return nil, io.EOF
			} else if count > len(msgs) {
				t.Errorf("recv() called %d times for %d messages", count, len(msgs))
			}
			count++
			msg := msgs[count-1]
			return msg.pb, msg.err
		}
	}
	ctx := runtime.NewServerMetadataContext(context.Background(), runtime.ServerMetadata{})
	marshaler := &CustomMarshaler{&runtime.JSONPb{}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recv := newTestRecv(t, tt.msgs)
			req := httptest.NewRequest("GET", "http://example.com/foo", nil)
			resp := httptest.NewRecorder()

			runtime.ForwardResponseStream(ctx, runtime.NewServeMux(), marshaler, resp, req, recv)

			w := resp.Result()
			if w.StatusCode != tt.statusCode {
				t.Errorf("StatusCode %d want %d", w.StatusCode, tt.statusCode)
			}
			if h := w.Header.Get("Transfer-Encoding"); h != "chunked" {
				t.Errorf("ForwardResponseStream missing header chunked")
			}
			body, err := ioutil.ReadAll(w.Body)
			if err != nil {
				t.Errorf("Failed to read response body with %v", err)
			}
			w.Body.Close()

			var want []byte
			for _, msg := range tt.msgs {
				if msg.err != nil {
					t.Skip("checking erorr encodings")
				}
				b, err := marshaler.Marshal(map[string]proto.Message{"result": msg.pb})
				if err != nil {
					t.Errorf("marshaler.Marshal() failed %v", err)
				}
				want = append(want, b...)
				want = append(want, "\n"...)
			}

			if string(body) != string(want) {
				t.Errorf("ForwardResponseStream() = \"%s\" want \"%s\"", body, want)
			}
		})
	}
}
