package server

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/prometheus/prometheus/prompb"

	"github.com/prometheus/prometheus/pp/go/transport"
)

const ProtocolVersion uint8 = 3

// TCPReader - wrappers over connection from cient.
type TCPReader struct {
	cfg     *transport.Config
	tt      *transport.Transport
	decoder *Decoder
	workDir string
}

// NewTCPReader - init new TCPReader.
func NewTCPReader(
	ctx context.Context,
	cfg *transport.Config,
	conn net.Conn,
) (*TCPReader, error) {
	return &TCPReader{
		cfg:     cfg,
		tt:      transport.New(cfg, conn),
		decoder: NewDecoder(),
	}, nil
}

// Authorization - incoming connection authorization.
func (r *TCPReader) Authorization(ctx context.Context) (*transport.AuthMsg, error) {
	raw, err := r.tt.Read(ctx)
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

// SendResponse - send response to client.
func (r *TCPReader) SendResponse(ctx context.Context, text string, code, segmentID uint32) error {
	resp := &transport.ResponseMsg{
		Text:      text,
		Code:      code,
		SegmentID: segmentID,
	}

	return r.tt.Write(ctx, transport.NewRawMessage(
		ProtocolVersion,
		transport.MsgResponse,
		resp.EncodeBinary(),
	))
}

// Next - read connection, decoding message and return protobuf in byte.
func (r *TCPReader) Next(ctx context.Context) (uint32, *prompb.WriteRequest, error) {
	for {
		raw, err := r.tt.Read(ctx)
		if err != nil {
			return 0, nil, err
		}

		switch raw.Header.Type {
		case transport.MsgSnapshot:
			err := r.decoder.Snapshot(ctx, raw.Payload)
			if err != nil {
				return 0, nil, err
			}
			continue
		case transport.MsgPut:
			blob, segmentID, err := r.decoder.Decode(ctx, raw.Payload)
			if err != nil {
				return 0, nil, err
			}

			msg := new(prompb.WriteRequest)
			err = msg.Unmarshal(blob.Bytes())
			blob.Destroy()
			if err != nil {
				return segmentID, nil, fmt.Errorf("unmarshal protobuf: %w", err)
			}
			return segmentID, msg, nil
		case transport.MsgRefill:
			// read refill and convert
			return 0, nil, errors.New("Not implemented")
		default:
			return 0, nil, fmt.Errorf("unknown msg type %d", raw.Header.Type)
		}
	}
}

/*
// RefillHandle -
func (r *TCPReader) RefillHandle(raw *delivery.RawMessage, s3sFn s3SenderFn) error {
	var refillMsg delivery.RefillMsg
	refillMsg.DecodeBinary(raw.Payload)

	dir, err := r.initDir(cfs)
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	// loop with processed read

	return nil
}

func (r *TCPReader) initDir() (string, error) {
	dir := filepath.Join(r.workDir, cfs.GetRequestID())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return dir, nil
}
*/

// Close - close TCPReader.
func (r *TCPReader) Close() error {
	return r.tt.Close()
}
