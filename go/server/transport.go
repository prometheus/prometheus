package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/prometheus/prometheus/prompb"
	"golang.org/x/net/websocket"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/transport"
)

const (
	protocolVersion       uint8 = 3
	protocolVersionSocket uint8 = 4
)

// Reader - implementation of the reader with the Next iterator function.
type Reader interface {
	Next(ctx context.Context) (*frames.ReadFrame, error)
}

// ProtocolReader - reader that reads raw messages and decodes them into WriteRequest.
type ProtocolReader struct {
	reader  Reader
	decoder *cppbridge.WALDecoder
}

// NewProtocolReader - init new ProtocolReader.
func NewProtocolReader(r Reader) *ProtocolReader {
	return &ProtocolReader{
		reader: r,
	}
}

// Next - func-iterator, read from reader raw message and return WriteRequest.
func (pr *ProtocolReader) Next(ctx context.Context) (*Request, error) {
	for {
		fe, err := pr.reader.Next(ctx)
		if err != nil {
			return nil, err
		}

		switch fe.GetType() {
		case frames.SegmentType:
			return pr.handlePut(ctx, fe)
		case frames.FinalType:
			return pr.handleFinal(fe)
		default:
			return nil, fmt.Errorf("unexpected msg type %d", fe.GetType())
		}
	}
}

// handlePut - process the put using the decoder.
func (pr *ProtocolReader) handlePut(ctx context.Context, fe *frames.ReadFrame) (*Request, error) {
	if pr.decoder == nil {
		pr.decoder = cppbridge.NewWALDecoder()
	}

	segment, err := pr.decoder.Decode(ctx, fe.GetBody())
	if err != nil {
		return nil, fmt.Errorf("decode segment: %w", err)
	}

	rq := &Request{
		SegmentID: segment.SegmentID(),
		Message:   new(prompb.WriteRequest),
		SentAt:    fe.GetCreatedAt(),
		CreatedAt: segment.CreatedAt(),
		EncodedAt: segment.EncodedAt(),
	}
	if err := segment.UnmarshalTo(rq.Message); err != nil {
		return rq, fmt.Errorf("unmarshal protobuf: %w", err)
	}
	return rq, nil
}

// handleFinal - process the final frame.
func (*ProtocolReader) handleFinal(rf *frames.ReadFrame) (*Request, error) {
	fm := frames.NewFinalMsgEmpty()
	if err := fm.UnmarshalBinary(rf.GetBody()); err != nil {
		return nil, err
	}
	return &Request{
		SegmentID: 0,
		SentAt:    rf.GetCreatedAt(),
		Finalized: true,
		HasRefill: fm.HasRefill(),
	}, nil
}

// TCPReader - wrappers over connection from cient.
type TCPReader struct {
	t *transport.NetTransport
}

var _ Reader = &TCPReader{}

// NewTCPReader - init new TCPReader.
func NewTCPReader(
	cfg *transport.Config,
	conn net.Conn,
) *TCPReader {
	t := transport.NewNetTransport(cfg, conn)
	return &TCPReader{t: t}
}

// Authorization - incoming connection authorization.
func (r *TCPReader) Authorization(ctx context.Context) (*frames.AuthMsg, error) {
	fe, err := r.t.Read(ctx)
	if err != nil {
		return nil, err
	}
	if fe.GetType() != frames.AuthType {
		return nil, fmt.Errorf("unexpected msg type %d", fe.GetType())
	}

	am := frames.NewAuthMsgEmpty()
	if err := am.UnmarshalBinary(fe.GetBody()); err != nil {
		return nil, err
	}
	return am, am.Validate()
}

// Next - func-iterator, read from conn and return raw message.
func (r *TCPReader) Next(ctx context.Context) (*frames.ReadFrame, error) {
	return r.t.Read(ctx)
}

// SendResponse - send response to client.
func (r *TCPReader) SendResponse(ctx context.Context, msg *frames.ResponseMsg) error {
	fr, err := frames.NewResponseFrameWithMsg(protocolVersion, msg)
	if err != nil {
		return err
	}
	return r.t.Write(ctx, fr)
}

// StartWithReader - special reader for read with start message.
type StartWithReader struct {
	Reader
	firstFrame *frames.ReadFrame
}

var _ Reader = &StartWithReader{}

// StartWith - init new StartWith Reader.
func StartWith(r Reader, fe *frames.ReadFrame) *StartWithReader {
	return &StartWithReader{
		Reader:     r,
		firstFrame: fe,
	}
}

// Next - func-iterator, read raw message from reader with start message.
func (swr *StartWithReader) Next(ctx context.Context) (*frames.ReadFrame, error) {
	if swr.firstFrame != nil {
		res := swr.firstFrame
		swr.firstFrame = nil
		return res, nil
	}
	return swr.Reader.Next(ctx)
}

// FileReader - reader raw messages from file.
type FileReader struct {
	file *os.File
}

var _ Reader = &FileReader{}

// NewFileReader - init new FileReader.
func NewFileReader(filePath string) (*FileReader, error) {
	file, err := os.OpenFile(
		filepath.Clean(filePath),
		os.O_RDONLY,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("open file %s: %w", filePath, err)
	}

	return &FileReader{
		file: file,
	}, nil
}

// Next - func-iterator, read from file and return raw message.
func (fr *FileReader) Next(ctx context.Context) (*frames.ReadFrame, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	fe := frames.NewFrameEmpty()
	if err := fe.Read(ctx, fr.file); err != nil {
		return nil, fmt.Errorf("read file %s: %w", fr.file.Name(), err)
	}

	return fe, nil
}

// Close - close the file reader.
func (fr *FileReader) Close() error {
	return fr.file.Close()
}

// Request - incoming request.
type Request struct {
	SegmentID uint32
	Message   *prompb.WriteRequest
	SentAt    int64
	CreatedAt int64
	EncodedAt int64
	Finalized bool
	HasRefill bool
}

// WebSocketReader - wrappers over connection from cient.
type WebSocketReader struct {
	t *transport.WebSocketTransport
}

// NewWebSocketReader - init new WebSocketReader.
func NewWebSocketReader(cfg *transport.Config, wsconn *websocket.Conn) *WebSocketReader {
	return &WebSocketReader{t: transport.NewWebSocketTransport(cfg, wsconn)}
}

// Next - func-iterator, read from conn and return raw message.
func (r *WebSocketReader) Next(ctx context.Context, fr frames.FrameReader) error {
	return r.t.Read(ctx, fr)
}

// SendResponse - send response to client.
func (r *WebSocketReader) SendResponse(ctx context.Context, msg *frames.ResponseV4) error {
	_, err := msg.WriteTo(r.t.Writer(ctx))
	return err
}

// FileReaderV4 - reader refill from file.
type FileReaderV4 struct {
	file *os.File
}

// NewFileReaderV4 - init new FileReaderV4.
func NewFileReaderV4(filePath string) (*FileReaderV4, error) {
	file, err := os.OpenFile(
		filepath.Clean(filePath),
		os.O_RDONLY,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("open file %s: %w", filePath, err)
	}

	return &FileReaderV4{
		file: file,
	}, nil
}

// Next - func-iterator, read from file and return raw message.
func (fr *FileReaderV4) Next(ctx context.Context) (*frames.ReadRefillSegmentV4, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	fe := frames.NewReadRefillSegmentV4Empty()
	if err := fe.Read(ctx, fr.file); err != nil {
		return nil, fmt.Errorf("read file %s: %w", fr.file.Name(), err)
	}

	return fe, nil
}

// Close - close the file reader.
func (fr *FileReaderV4) Close() error {
	return fr.file.Close()
}
