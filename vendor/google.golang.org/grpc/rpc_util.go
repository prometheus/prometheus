/*
 *
 * Copyright 2014, Google Inc.
 * All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions are
 * met:
 *
 *     * Redistributions of source code must retain the above copyright
 * notice, this list of conditions and the following disclaimer.
 *     * Redistributions in binary form must reproduce the above
 * copyright notice, this list of conditions and the following disclaimer
 * in the documentation and/or other materials provided with the
 * distribution.
 *     * Neither the name of Google Inc. nor the names of its
 * contributors may be used to endorse or promote products derived from
 * this software without specific prior written permission.
 *
 * THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
 * "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
 * LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
 * A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
 * OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
 * SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
 * LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
 * DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
 * THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
 * (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
 * OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 *
 */

package grpc

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"io/ioutil"
	"math"
	"os"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/transport"
)

// Compressor defines the interface gRPC uses to compress a message.
type Compressor interface {
	// Do compresses p into w.
	Do(w io.Writer, p []byte) error
	// Type returns the compression algorithm the Compressor uses.
	Type() string
}

type gzipCompressor struct {
	pool sync.Pool
}

// NewGZIPCompressor creates a Compressor based on GZIP.
func NewGZIPCompressor() Compressor {
	return &gzipCompressor{
		pool: sync.Pool{
			New: func() interface{} {
				return gzip.NewWriter(ioutil.Discard)
			},
		},
	}
}

func (c *gzipCompressor) Do(w io.Writer, p []byte) error {
	z := c.pool.Get().(*gzip.Writer)
	z.Reset(w)
	if _, err := z.Write(p); err != nil {
		return err
	}
	return z.Close()
}

func (c *gzipCompressor) Type() string {
	return "gzip"
}

// Decompressor defines the interface gRPC uses to decompress a message.
type Decompressor interface {
	// Do reads the data from r and uncompress them.
	Do(r io.Reader) ([]byte, error)
	// Type returns the compression algorithm the Decompressor uses.
	Type() string
}

type gzipDecompressor struct {
	pool sync.Pool
}

// NewGZIPDecompressor creates a Decompressor based on GZIP.
func NewGZIPDecompressor() Decompressor {
	return &gzipDecompressor{}
}

func (d *gzipDecompressor) Do(r io.Reader) ([]byte, error) {
	var z *gzip.Reader
	switch maybeZ := d.pool.Get().(type) {
	case nil:
		newZ, err := gzip.NewReader(r)
		if err != nil {
			return nil, err
		}
		z = newZ
	case *gzip.Reader:
		z = maybeZ
		if err := z.Reset(r); err != nil {
			d.pool.Put(z)
			return nil, err
		}
	}

	defer func() {
		z.Close()
		d.pool.Put(z)
	}()
	return ioutil.ReadAll(z)
}

func (d *gzipDecompressor) Type() string {
	return "gzip"
}

// callInfo contains all related configuration and information about an RPC.
type callInfo struct {
	failFast  bool
	headerMD  metadata.MD
	trailerMD metadata.MD
	peer      *peer.Peer
	traceInfo traceInfo // in trace.go
	creds     credentials.PerRPCCredentials
}

var defaultCallInfo = callInfo{failFast: true}

// CallOption configures a Call before it starts or extracts information from
// a Call after it completes.
type CallOption interface {
	// before is called before the call is sent to any server.  If before
	// returns a non-nil error, the RPC fails with that error.
	before(*callInfo) error

	// after is called after the call has completed.  after cannot return an
	// error, so any failures should be reported via output parameters.
	after(*callInfo)
}

type beforeCall func(c *callInfo) error

func (o beforeCall) before(c *callInfo) error { return o(c) }
func (o beforeCall) after(c *callInfo)        {}

type afterCall func(c *callInfo)

func (o afterCall) before(c *callInfo) error { return nil }
func (o afterCall) after(c *callInfo)        { o(c) }

// Header returns a CallOptions that retrieves the header metadata
// for a unary RPC.
func Header(md *metadata.MD) CallOption {
	return afterCall(func(c *callInfo) {
		*md = c.headerMD
	})
}

// Trailer returns a CallOptions that retrieves the trailer metadata
// for a unary RPC.
func Trailer(md *metadata.MD) CallOption {
	return afterCall(func(c *callInfo) {
		*md = c.trailerMD
	})
}

// Peer returns a CallOption that retrieves peer information for a
// unary RPC.
func Peer(peer *peer.Peer) CallOption {
	return afterCall(func(c *callInfo) {
		if c.peer != nil {
			*peer = *c.peer
		}
	})
}

// FailFast configures the action to take when an RPC is attempted on broken
// connections or unreachable servers. If failfast is true, the RPC will fail
// immediately. Otherwise, the RPC client will block the call until a
// connection is available (or the call is canceled or times out) and will retry
// the call if it fails due to a transient error. Please refer to
// https://github.com/grpc/grpc/blob/master/doc/wait-for-ready.md.
// Note: failFast is default to true.
func FailFast(failFast bool) CallOption {
	return beforeCall(func(c *callInfo) error {
		c.failFast = failFast
		return nil
	})
}

// PerRPCCredentials returns a CallOption that sets credentials.PerRPCCredentials
// for a call.
func PerRPCCredentials(creds credentials.PerRPCCredentials) CallOption {
	return beforeCall(func(c *callInfo) error {
		c.creds = creds
		return nil
	})
}

// The format of the payload: compressed or not?
type payloadFormat uint8

const (
	compressionNone payloadFormat = iota // no compression
	compressionMade
)

// parser reads complete gRPC messages from the underlying reader.
type parser struct {
	// r is the underlying reader.
	// See the comment on recvMsg for the permissible
	// error types.
	r io.Reader

	// The header of a gRPC message. Find more detail
	// at http://www.grpc.io/docs/guides/wire.html.
	header [5]byte
}

// recvMsg reads a complete gRPC message from the stream.
//
// It returns the message and its payload (compression/encoding)
// format. The caller owns the returned msg memory.
//
// If there is an error, possible values are:
//   * io.EOF, when no messages remain
//   * io.ErrUnexpectedEOF
//   * of type transport.ConnectionError
//   * of type transport.StreamError
// No other error values or types must be returned, which also means
// that the underlying io.Reader must not return an incompatible
// error.
func (p *parser) recvMsg(maxMsgSize int) (pf payloadFormat, msg []byte, err error) {
	if _, err := io.ReadFull(p.r, p.header[:]); err != nil {
		return 0, nil, err
	}

	pf = payloadFormat(p.header[0])
	length := binary.BigEndian.Uint32(p.header[1:])

	if length == 0 {
		return pf, nil, nil
	}
	if length > uint32(maxMsgSize) {
		return 0, nil, Errorf(codes.Internal, "grpc: received message length %d exceeding the max size %d", length, maxMsgSize)
	}
	// TODO(bradfitz,zhaoq): garbage. reuse buffer after proto decoding instead
	// of making it for each message:
	msg = make([]byte, int(length))
	if _, err := io.ReadFull(p.r, msg); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return 0, nil, err
	}
	return pf, msg, nil
}

// encode serializes msg and prepends the message header. If msg is nil, it
// generates the message header of 0 message length.
func encode(c Codec, msg interface{}, cp Compressor, cbuf *bytes.Buffer, outPayload *stats.OutPayload) ([]byte, error) {
	var (
		b      []byte
		length uint
	)
	if msg != nil {
		var err error
		// TODO(zhaoq): optimize to reduce memory alloc and copying.
		b, err = c.Marshal(msg)
		if err != nil {
			return nil, err
		}
		if outPayload != nil {
			outPayload.Payload = msg
			// TODO truncate large payload.
			outPayload.Data = b
			outPayload.Length = len(b)
		}
		if cp != nil {
			if err := cp.Do(cbuf, b); err != nil {
				return nil, err
			}
			b = cbuf.Bytes()
		}
		length = uint(len(b))
	}
	if length > math.MaxUint32 {
		return nil, Errorf(codes.InvalidArgument, "grpc: message too large (%d bytes)", length)
	}

	const (
		payloadLen = 1
		sizeLen    = 4
	)

	var buf = make([]byte, payloadLen+sizeLen+len(b))

	// Write payload format
	if cp == nil {
		buf[0] = byte(compressionNone)
	} else {
		buf[0] = byte(compressionMade)
	}
	// Write length of b into buf
	binary.BigEndian.PutUint32(buf[1:], uint32(length))
	// Copy encoded msg to buf
	copy(buf[5:], b)

	if outPayload != nil {
		outPayload.WireLength = len(buf)
	}

	return buf, nil
}

func checkRecvPayload(pf payloadFormat, recvCompress string, dc Decompressor) error {
	switch pf {
	case compressionNone:
	case compressionMade:
		if dc == nil || recvCompress != dc.Type() {
			return Errorf(codes.Unimplemented, "grpc: Decompressor is not installed for grpc-encoding %q", recvCompress)
		}
	default:
		return Errorf(codes.Internal, "grpc: received unexpected payload format %d", pf)
	}
	return nil
}

func recv(p *parser, c Codec, s *transport.Stream, dc Decompressor, m interface{}, maxMsgSize int, inPayload *stats.InPayload) error {
	pf, d, err := p.recvMsg(maxMsgSize)
	if err != nil {
		return err
	}
	if inPayload != nil {
		inPayload.WireLength = len(d)
	}
	if err := checkRecvPayload(pf, s.RecvCompress(), dc); err != nil {
		return err
	}
	if pf == compressionMade {
		d, err = dc.Do(bytes.NewReader(d))
		if err != nil {
			return Errorf(codes.Internal, "grpc: failed to decompress the received message %v", err)
		}
	}
	if len(d) > maxMsgSize {
		// TODO: Revisit the error code. Currently keep it consistent with java
		// implementation.
		return Errorf(codes.Internal, "grpc: received a message of %d bytes exceeding %d limit", len(d), maxMsgSize)
	}
	if err := c.Unmarshal(d, m); err != nil {
		return Errorf(codes.Internal, "grpc: failed to unmarshal the received message %v", err)
	}
	if inPayload != nil {
		inPayload.RecvTime = time.Now()
		inPayload.Payload = m
		// TODO truncate large payload.
		inPayload.Data = d
		inPayload.Length = len(d)
	}
	return nil
}

type rpcInfo struct {
	bytesSent     bool
	bytesReceived bool
}

type rpcInfoContextKey struct{}

func newContextWithRPCInfo(ctx context.Context) context.Context {
	return context.WithValue(ctx, rpcInfoContextKey{}, &rpcInfo{})
}

func rpcInfoFromContext(ctx context.Context) (s *rpcInfo, ok bool) {
	s, ok = ctx.Value(rpcInfoContextKey{}).(*rpcInfo)
	return
}

func updateRPCInfoInContext(ctx context.Context, s rpcInfo) {
	if ss, ok := rpcInfoFromContext(ctx); ok {
		*ss = s
	}
	return
}

// Code returns the error code for err if it was produced by the rpc system.
// Otherwise, it returns codes.Unknown.
//
// Deprecated; use status.FromError and Code method instead.
func Code(err error) codes.Code {
	if s, ok := status.FromError(err); ok {
		return s.Code()
	}
	return codes.Unknown
}

// ErrorDesc returns the error description of err if it was produced by the rpc system.
// Otherwise, it returns err.Error() or empty string when err is nil.
//
// Deprecated; use status.FromError and Message method instead.
func ErrorDesc(err error) string {
	if s, ok := status.FromError(err); ok {
		return s.Message()
	}
	return err.Error()
}

// Errorf returns an error containing an error code and a description;
// Errorf returns nil if c is OK.
//
// Deprecated; use status.Errorf instead.
func Errorf(c codes.Code, format string, a ...interface{}) error {
	return status.Errorf(c, format, a...)
}

// toRPCErr converts an error into an error from the status package.
func toRPCErr(err error) error {
	if _, ok := status.FromError(err); ok {
		return err
	}
	switch e := err.(type) {
	case transport.StreamError:
		return status.Error(e.Code, e.Desc)
	case transport.ConnectionError:
		return status.Error(codes.Internal, e.Desc)
	default:
		switch err {
		case context.DeadlineExceeded:
			return status.Error(codes.DeadlineExceeded, err.Error())
		case context.Canceled:
			return status.Error(codes.Canceled, err.Error())
		case ErrClientConnClosing:
			return status.Error(codes.FailedPrecondition, err.Error())
		}
	}
	return status.Error(codes.Unknown, err.Error())
}

// convertCode converts a standard Go error into its canonical code. Note that
// this is only used to translate the error returned by the server applications.
func convertCode(err error) codes.Code {
	switch err {
	case nil:
		return codes.OK
	case io.EOF:
		return codes.OutOfRange
	case io.ErrClosedPipe, io.ErrNoProgress, io.ErrShortBuffer, io.ErrShortWrite, io.ErrUnexpectedEOF:
		return codes.FailedPrecondition
	case os.ErrInvalid:
		return codes.InvalidArgument
	case context.Canceled:
		return codes.Canceled
	case context.DeadlineExceeded:
		return codes.DeadlineExceeded
	}
	switch {
	case os.IsExist(err):
		return codes.AlreadyExists
	case os.IsNotExist(err):
		return codes.NotFound
	case os.IsPermission(err):
		return codes.PermissionDenied
	}
	return codes.Unknown
}

// MethodConfig defines the configuration recommended by the service providers for a
// particular method.
// This is EXPERIMENTAL and subject to change.
type MethodConfig struct {
	// WaitForReady indicates whether RPCs sent to this method should wait until
	// the connection is ready by default (!failfast). The value specified via the
	// gRPC client API will override the value set here.
	WaitForReady bool
	// Timeout is the default timeout for RPCs sent to this method. The actual
	// deadline used will be the minimum of the value specified here and the value
	// set by the application via the gRPC client API.  If either one is not set,
	// then the other will be used.  If neither is set, then the RPC has no deadline.
	Timeout time.Duration
	// MaxReqSize is the maximum allowed payload size for an individual request in a
	// stream (client->server) in bytes. The size which is measured is the serialized
	// payload after per-message compression (but before stream compression) in bytes.
	// The actual value used is the minumum of the value specified here and the value set
	// by the application via the gRPC client API. If either one is not set, then the other
	// will be used.  If neither is set, then the built-in default is used.
	// TODO: support this.
	MaxReqSize uint32
	// MaxRespSize is the maximum allowed payload size for an individual response in a
	// stream (server->client) in bytes.
	// TODO: support this.
	MaxRespSize uint32
}

// ServiceConfig is provided by the service provider and contains parameters for how
// clients that connect to the service should behave.
// This is EXPERIMENTAL and subject to change.
type ServiceConfig struct {
	// LB is the load balancer the service providers recommends. The balancer specified
	// via grpc.WithBalancer will override this.
	LB Balancer
	// Methods contains a map for the methods in this service.
	Methods map[string]MethodConfig
}

// SupportPackageIsVersion4 is referenced from generated protocol buffer files
// to assert that that code is compatible with this version of the grpc package.
//
// This constant may be renamed in the future if a change in the generated code
// requires a synchronised update of grpc-go and protoc-gen-go. This constant
// should not be referenced from any other code.
const SupportPackageIsVersion4 = true

// Version is the current grpc version.
const Version = "1.4.0-dev"

const grpcUA = "grpc-go/" + Version
