package server

// import (
// 	"context"
// 	"fmt"
// 	"net"
// 	"os"
// 	"path/filepath"

// 	"github.com/prometheus/prometheus/prompb"

// 	"github.com/prometheus/prometheus/pp/go/transport"
// )

// // TODO DELETE after make e2e test
// // TCPReader - wrappers over connection from cient.
// type TCPReader struct {
// 	cfg        *transport.Config
// 	tt         *transport.Transport
// 	decoder    *Decoder
// 	incomeDir  string
// 	incomeFile string
// }

// // NewTCPReader - init new TCPReader.
// func NewTCPReader(
// 	cfg *transport.Config,
// 	conn net.Conn,
// ) (*TCPReader, error) {
// 	return &TCPReader{
// 		cfg:     cfg,
// 		tt:      transport.New(cfg, conn),
// 		decoder: NewDecoder(),
// 	}, nil
// }

// // Authorization - incoming connection authorization.
// func (r *TCPReader) Authorization(ctx context.Context) (*transport.AuthMsg, error) {
// 	raw, err := r.tt.Read(ctx)
// 	if err != nil {
// 		return nil, err
// 	}
// 	if raw.Header.Type != transport.MsgAuth {
// 		return nil, fmt.Errorf("unexpected msg type %d", raw.Header.Type)
// 	}

// 	am := new(transport.AuthMsg)
// 	am.DecodeBinary(raw.Payload)
// 	return am, am.Validate()
// }

// // SendResponse - send response to client.
// func (r *TCPReader) SendResponse(ctx context.Context, text string, code, segmentID uint32) error {
// 	resp := &transport.ResponseMsg{
// 		Text:      text,
// 		Code:      code,
// 		SegmentID: segmentID,
// 	}

// 	return r.tt.Write(ctx, transport.NewRawMessage(
// 		protocolVersion,
// 		transport.MsgResponse,
// 		resp.EncodeBinary(),
// 	))
// }

// // Next - read connection, decoding message and return protobuf in byte.
// func (r *TCPReader) Next(ctx context.Context) (uint32, *prompb.WriteRequest, error) {
// 	for {
// 		raw, err := r.tt.Read(ctx)
// 		if err != nil {
// 			return 0, nil, err
// 		}

// 		switch raw.Header.Type {
// 		case transport.MsgSnapshot:
// 			err := r.decoder.Snapshot(ctx, raw.Payload)
// 			if err != nil {
// 				return 0, nil, fmt.Errorf("decode snapshot: %w", err)
// 			}
// 		case transport.MsgDryPut:
// 			err := r.decoder.DecodeDry(ctx, raw.Payload)
// 			if err != nil {
// 				return 0, nil, fmt.Errorf("decode dry segment: %w", err)
// 			}
// 		case transport.MsgPut:
// 			blob, segmentID, err := r.decoder.Decode(ctx, raw.Payload)
// 			if err != nil {
// 				return 0, nil, fmt.Errorf("decode segment: %w", err)
// 			}

// 			msg := new(prompb.WriteRequest)
// 			err = msg.Unmarshal(blob.Bytes())
// 			blob.Destroy()
// 			if err != nil {
// 				return segmentID, nil, fmt.Errorf("unmarshal protobuf: %w", err)
// 			}
// 			return segmentID, msg, nil
// 		case transport.MsgRefill:
// 			msg, err := r.RefillHandle(ctx, raw)
// 			if err != nil {
// 				return 0, nil, fmt.Errorf("refill handle: %w", err)
// 			}
// 			return 0, msg, nil
// 		default:
// 			return 0, nil, fmt.Errorf("unknown msg type %d", raw.Header.Type)
// 		}
// 	}
// }

// // RefillHandle -
// func (r *TCPReader) RefillHandle(
// 	ctx context.Context,
// 	raw *transport.RawMessage,
// ) (*prompb.WriteRequest, error) {
// 	var refillMsg transport.RefillMsg
// 	if err := refillMsg.UnmarshalBinary(raw.Payload); err != nil {
// 		return nil, err
// 	}

// 	dir, err := r.initDir()
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer func() {
// 		_ = os.RemoveAll(dir)
// 	}()

// 	// refillMsg.NumberOfMessage
// 	fileName, err := r.saveRawRefill(ctx, dir, 0)
// 	if err != nil {
// 		return nil, err
// 	}

// 	msg, err := r.decodeRawRefill(ctx, fileName, 0)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return msg, nil
// }

// func (r *TCPReader) decodeRawRefill(
// 	ctx context.Context,
// 	fileName string,
// 	numberOfMessage uint32,
// ) (*prompb.WriteRequest, error) {
// 	// #nosec G302
// 	file, err := os.OpenFile(
// 		filepath.Clean(fileName),
// 		os.O_CREATE|os.O_RDONLY,
// 		0o644, //nolint:revive // not need constant
// 	)
// 	if err != nil {
// 		return nil, fmt.Errorf("open file %s: %w", fileName, err)
// 	}

// 	var recreateDecoder bool
// 	fullMsg := new(prompb.WriteRequest)
// 	decoder := NewDecoder()
// 	defer decoder.Destroy()

// 	for i := 0; i < int(numberOfMessage); i++ {
// 		raw, err := transport.ReadRawMessage(file)
// 		if err != nil {
// 			return nil, fmt.Errorf("read file %s: %w", fileName, err)
// 		}

// 		switch raw.Header.Type {
// 		case transport.MsgSnapshot:
// 			if recreateDecoder {
// 				decoder.Destroy()
// 				decoder = NewDecoder()
// 			}

// 			if err := decoder.Snapshot(ctx, raw.Payload); err != nil {
// 				return nil, fmt.Errorf("decode snapshot: %w", err)
// 			}
// 			recreateDecoder = true
// 		case transport.MsgDryPut:
// 			if recreateDecoder {
// 				recreateDecoder = false
// 			}

// 			if err := decoder.DecodeDry(ctx, raw.Payload); err != nil {
// 				return nil, fmt.Errorf("decode dry segment: %w", err)
// 			}
// 		case transport.MsgPut:
// 			if recreateDecoder {
// 				recreateDecoder = false
// 			}

// 			blob, _, err := decoder.Decode(ctx, raw.Payload)
// 			if err != nil {
// 				return nil, fmt.Errorf("decode segment: %w", err)
// 			}

// 			msg := new(prompb.WriteRequest)
// 			err = msg.Unmarshal(blob.Bytes())
// 			blob.Destroy()
// 			if err != nil {
// 				return nil, fmt.Errorf("unmarshal protobuf: %w", err)
// 			}

// 			fullMsg.Timeseries = append(fullMsg.Timeseries, msg.Timeseries...)
// 			fullMsg.Metadata = append(fullMsg.Metadata, msg.Metadata...)
// 		default:
// 			return nil, fmt.Errorf("unknown msg type %d", raw.Header.Type)
// 		}
// 	}

// 	return fullMsg, nil
// }

// func (r *TCPReader) saveRawRefill(
// 	ctx context.Context,
// 	dir string,
// 	numberOfMessage uint32,
// ) (string, error) {
// 	file, err := os.CreateTemp(dir, r.incomeFile)
// 	if err != nil {
// 		return "", fmt.Errorf("open file: %w", err)
// 	}
// 	defer func() {
// 		_ = file.Close()
// 	}()

// 	var raw *transport.RawMessage
// 	for i := 0; i < int(numberOfMessage); i++ {
// 		raw, err = r.tt.Read(ctx)
// 		if err != nil {
// 			return "", err
// 		}

// 		switch raw.Header.Type {
// 		case transport.MsgSnapshot, transport.MsgDryPut, transport.MsgPut:
// 			if err = transport.WriteRawMessage(file, raw); err != nil {
// 				return "", err
// 			}
// 		default:
// 			return "", fmt.Errorf("unknown msg type %d", raw.Header.Type)
// 		}
// 	}

// 	if err = file.Sync(); err != nil {
// 		return "", fmt.Errorf("sync file: %w", err)
// 	}

// 	return file.Name(), nil
// }

// func (r *TCPReader) initDir() (string, error) {
// 	dir, err := os.MkdirTemp("", fmt.Sprintf("%s-", r.incomeDir))
// 	if err != nil {
// 		return "", fmt.Errorf("mkdir %s: %w", dir, err)
// 	}

// 	return dir, nil
// }

// // Close - close TCPReader.
// func (r *TCPReader) Close() error {
// 	return r.tt.Close()
// }
