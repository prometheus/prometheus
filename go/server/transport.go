package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/prometheus/prometheus/prompb"

	"github.com/prometheus/prometheus/pp/go/common"
	"github.com/prometheus/prometheus/pp/go/transport"
)

const protocolVersion uint8 = 3

// Reader - implementation of the reader with the Next iterator function.
type Reader interface {
	Next(ctx context.Context) (*transport.RawMessage, error)
}

// ProtocolReader - reader that reads raw messages and decodes them into WriteRequest.
type ProtocolReader struct {
	reader  Reader
	decoder *common.Decoder
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
		raw, err := pr.reader.Next(ctx)
		if err != nil {
			return nil, err
		}

		switch raw.Header.Type {
		case transport.MsgSnapshot:
			if err := pr.handleSnapshot(ctx, raw.Payload); err != nil {
				return nil, fmt.Errorf("decode snapshot: %w", err)
			}
		case transport.MsgDryPut:
			if err := pr.handleDryPut(ctx, raw.Payload); err != nil {
				return nil, fmt.Errorf("decode dry segment: %w", err)
			}
		case transport.MsgPut:
			return pr.handlePut(ctx, raw)
		default:
			return nil, fmt.Errorf("unexpected msg type %d", raw.Header.Type)
		}
	}
}

// Destroy - destroy the decoder reader.
func (pr *ProtocolReader) Destroy() {
	if pr.decoder != nil {
		pr.decoder.Destroy()
	}
}

// handleSnapshot - process the snapshot using the decoder.
func (pr *ProtocolReader) handleSnapshot(ctx context.Context, snapshot []byte) error {
	if pr.decoder != nil {
		pr.decoder.Destroy()
	}
	pr.decoder = common.NewDecoder()
	return pr.decoder.Snapshot(ctx, snapshot)
}

// handleDryPut - process the dry put using the decoder.
func (pr *ProtocolReader) handleDryPut(ctx context.Context, segment []byte) error {
	if pr.decoder == nil {
		pr.decoder = common.NewDecoder()
	}
	return pr.decoder.DecodeDry(ctx, segment)
}

// handlePut - process the put using the decoder.
func (pr *ProtocolReader) handlePut(ctx context.Context, raw *transport.RawMessage) (*Request, error) {
	if pr.decoder == nil {
		pr.decoder = common.NewDecoder()
	}
	blob, segmentID, err := pr.decoder.Decode(ctx, raw.Payload)
	if err != nil {
		return nil, fmt.Errorf("decode segment: %w", err)
	}
	defer blob.Destroy()

	rq := &Request{
		SegmentID: segmentID,
		Message:   new(prompb.WriteRequest),
		SentAt:    raw.Header.CreatedAt,
		CreatedAt: blob.CreatedAt(),
		EncodedAt: blob.EncodedAt(),
	}
	if err := rq.Message.Unmarshal(blob.Bytes()); err != nil {
		return rq, fmt.Errorf("unmarshal protobuf: %w", err)
	}
	return rq, nil
}

// TCPReader - wrappers over connection from cient.
type TCPReader struct {
	t *transport.Transport
}

var _ Reader = &TCPReader{}

// NewTCPReader - init new TCPReader.
func NewTCPReader(
	cfg *transport.Config,
	conn net.Conn,
) *TCPReader {
	t := transport.New(cfg, conn)
	return &TCPReader{t: t}
}

// Authorization - incoming connection authorization.
func (r *TCPReader) Authorization(ctx context.Context) (*transport.AuthMsg, error) {
	raw, err := r.t.Read(ctx)
	if err != nil {
		return nil, err
	}
	if raw.Header.Type != transport.MsgAuth {
		return nil, fmt.Errorf("unexpected msg type %d", raw.Header.Type)
	}

	am := new(transport.AuthMsg)
	am.DecodeBinary(raw.Payload)
	return am, am.Validate()
}

// Next - func-iterator, read from conn and return raw message.
func (r *TCPReader) Next(ctx context.Context) (*transport.RawMessage, error) {
	return r.t.Read(ctx)
}

// SendResponse - send response to client.
func (r *TCPReader) SendResponse(ctx context.Context, msg *transport.ResponseMsg) error {
	return r.t.Write(ctx, transport.NewRawMessage(
		protocolVersion,
		transport.MsgResponse,
		msg.EncodeBinary(),
	))
}

// StartWithReader - special reader for read with start message.
type StartWithReader struct {
	Reader
	firstMsg *transport.RawMessage
}

var _ Reader = &StartWithReader{}

// StartWith - init new StartWith Reader.
func StartWith(r Reader, msg *transport.RawMessage) *StartWithReader {
	return &StartWithReader{
		Reader:   r,
		firstMsg: msg,
	}
}

// Next - func-iterator, read raw message from reader with start message.
func (swr *StartWithReader) Next(ctx context.Context) (*transport.RawMessage, error) {
	if swr.firstMsg != nil {
		res := swr.firstMsg
		swr.firstMsg = nil
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
func (fr *FileReader) Next(ctx context.Context) (*transport.RawMessage, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	raw, err := transport.ReadRawMessage(fr.file)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", fr.file.Name(), err)
	}

	return raw, nil
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
}
