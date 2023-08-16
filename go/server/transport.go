package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/prometheus/prometheus/prompb"

	"github.com/prometheus/prometheus/pp/go/common"
	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/transport"
)

const protocolVersion uint8 = 3

// Reader - implementation of the reader with the Next iterator function.
type Reader interface {
	Next(ctx context.Context) (*frames.ReadFrame, error)
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
		fe, err := pr.reader.Next(ctx)
		if err != nil {
			return nil, err
		}

		switch fe.GetType() {
		case frames.SnapshotType:
			if err := pr.handleSnapshot(ctx, fe); err != nil {
				return nil, fmt.Errorf("decode snapshot: %w", err)
			}
		case frames.DrySegmentType:
			if err := pr.handleDryPut(ctx, fe); err != nil {
				return nil, fmt.Errorf("decode dry segment: %w", err)
			}
		case frames.SegmentType:
			return pr.handlePut(ctx, fe)
		default:
			return nil, fmt.Errorf("unexpected msg type %d", fe.GetType())
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
func (pr *ProtocolReader) handleSnapshot(ctx context.Context, fe *frames.ReadFrame) error {
	if pr.decoder != nil {
		pr.decoder.Destroy()
	}
	var err error
	pr.decoder, err = common.NewDecoder()
	if err != nil {
		return err
	}
	return pr.decoder.Snapshot(ctx, fe.GetBody())
}

// handleDryPut - process the dry put using the decoder.
func (pr *ProtocolReader) handleDryPut(ctx context.Context, fe *frames.ReadFrame) error {
	if pr.decoder == nil {
		var err error
		pr.decoder, err = common.NewDecoder()
		if err != nil {
			return err
		}
	}
	return pr.decoder.DecodeDry(ctx, fe.GetBody())
}

// handlePut - process the put using the decoder.
func (pr *ProtocolReader) handlePut(ctx context.Context, fe *frames.ReadFrame) (*Request, error) {
	if pr.decoder == nil {
		var err error
		pr.decoder, err = common.NewDecoder()
		if err != nil {
			return nil, fmt.Errorf("create new decoder for segment: %w", err)
		}
	}

	blob, segmentID, err := pr.decoder.Decode(ctx, fe.GetBody())
	if err != nil {
		return nil, fmt.Errorf("decode segment: %w", err)
	}

	rq := &Request{
		SegmentID: segmentID,
		Message:   new(prompb.WriteRequest),
		SentAt:    fe.GetCreatedAt(),
		CreatedAt: blob.CreatedAt(),
		EncodedAt: blob.EncodedAt(),
	}
	if err := blob.UnmarshalTo(rq.Message); err != nil {
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
}
