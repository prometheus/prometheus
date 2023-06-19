package server

// import (
// 	"context"
// 	"errors"
// 	"fmt"
// 	"io"
// 	"net"
// 	"os"

// 	"github.com/prometheus/prometheus/prompb"

// 	"github.com/prometheus/prometheus/pp/go/transport"
// )

// // ErrUnknownMsgType - error for unknown msg type.
// type ErrUnknownMsgType struct {
// 	err     error
// 	msgType int8
// }

// // UnknownMsgType - init ErrUnknownMsgType.
// func UnknownMsgType(msgType int8) error {
// 	return ErrUnknownMsgType{
// 		err:     errors.New("unknown msg type"),
// 		msgType: msgType,
// 	}
// }

// // Error - implements error.
// func (e ErrUnknownMsgType) Error() string {
// 	return fmt.Sprintf("%s: %d", e.err, e.msgType)
// }

// // Unwrap - implements errors.Unwrap.
// func (e ErrUnknownMsgType) Unwrap() error {
// 	return e.err
// }

// // Is - implements errors.Is.
// func (ErrUnknownMsgType) Is(target error) bool {
// 	_, ok := target.(ErrUnknownMsgType)
// 	return ok
// }

// // ResultHandler - result output of the handler.
// type ResultHandler struct {
// 	Code      int
// 	SegmentID uint32
// 	Msg       string
// }

// // NewResultHandler - init new ResultHandler.
// func NewResultHandler(code int, segmentID uint32, msg string) *ResultHandler {
// 	return &ResultHandler{Code: code, SegmentID: segmentID, Msg: msg}
// }

// type (
// 	// NextWRFn - func-iterator for read WriteRequest's.
// 	NextWRFn func(context.Context) (uint32, *prompb.WriteRequest, error)

// 	// WriteRequestHandler - external handler for received WriteRequest messages.
// 	WriteRequestHandler func(context.Context, NextWRFn) (*ResultHandler, error)

// 	// IncomeHandler - incoming data handler.
// 	IncomeHandler func(ctx context.Context) (*ResultHandler, error)

// 	// nextRawFn - func-iterator for read RawMessage's.
// 	nextRawFn func(context.Context) (*transport.RawMessage, error)
// )

// // ManagerConfig - config for Manager.
// type ManagerConfig struct {
// 	TransportCfg *transport.Config
// 	PrefixDir    string
// 	PrefixFile   string
// }

// // Manager - server manager.
// type Manager struct {
// 	reader        *TCPReader2
// 	decoderReader *DecoderReader
// 	fileHandler   *FileHandler
// 	prefixDir     string
// 	prefixFile    string
// }

// // NewManager - init new Manager.
// func NewManager(mcfg *ManagerConfig, conn net.Conn) (*Manager, error) {
// 	treader, err := NewTCPReader2(mcfg.TransportCfg, conn)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return &Manager{
// 		reader:     treader,
// 		prefixDir:  mcfg.PrefixDir,
// 		prefixFile: mcfg.PrefixFile,
// 	}, nil
// }

// // Authorization - incoming connection authorization.
// func (m *Manager) Authorization(ctx context.Context) (*transport.AuthMsg, error) {
// 	return m.reader.Authorization(ctx)
// }

// // SendResponse - send response to client.
// func (m *Manager) SendResponse(ctx context.Context, text string, code, segmentID uint32) error {
// 	return m.reader.SendResponse(ctx, text, code, segmentID)
// }

// // Handler - main handler of the manager with recognition of incoming data, returns the corresponding handler.
// func (m *Manager) Handler(
// 	ctx context.Context,
// 	streamWrHandler, refillWrHandler WriteRequestHandler,
// ) (IncomeHandler, error) {
// 	raw, err := m.reader.Next(ctx)
// 	if err != nil {
// 		return nil, err
// 	}

// 	switch raw.Header.Type {
// 	case transport.MsgSnapshot, transport.MsgDryPut, transport.MsgPut:
// 		return m.createStreamHandle(raw, streamWrHandler), nil
// 	case transport.MsgRefill:
// 		return m.createRefillHandle(ctx, raw, refillWrHandler)
// 	default:
// 		return nil, UnknownMsgType(int8(raw.Header.Type))
// 	}
// }

// // createStreamHandle - create an incoming data handler for the stream.
// func (m *Manager) createStreamHandle(
// 	raw *transport.RawMessage,
// 	wrHandler WriteRequestHandler,
// ) IncomeHandler {
// 	m.decoderReader = NewDecoderReader(raw)
// 	next := func(ctx context.Context) (uint32, *prompb.WriteRequest, error) {
// 		return m.decoderReader.Next(ctx, m.reader.Next)
// 	}
// 	return func(ctx context.Context) (*ResultHandler, error) {
// 		return wrHandler(ctx, next)
// 	}
// }

// // createRefillHandle - create incoming data handler for refill.
// func (m *Manager) createRefillHandle(
// 	ctx context.Context,
// 	raw *transport.RawMessage,
// 	wrHandler WriteRequestHandler,
// ) (IncomeHandler, error) {
// 	var err error
// 	m.fileHandler, err = NewFileHandler(m.prefixDir, m.prefixFile)
// 	if err != nil {
// 		return nil, err
// 	}
// 	// read from connection and save file
// 	var refillMsg transport.RefillMsg
// 	if err = refillMsg.UnmarshalBinary(raw.Payload); err != nil {
// 		return nil, fmt.Errorf("unmarshal binary: %w", err)
// 	}

// 	if err = m.fileHandler.Copy(ctx, len(refillMsg.Messages), m.reader.Next); err != nil {
// 		return nil, fmt.Errorf("copy to file: %w", err)
// 	}
// 	// set to begin file for read
// 	if err = m.fileHandler.SeekStart(); err != nil {
// 		return nil, fmt.Errorf("seek file: %w", err)
// 	}
// 	m.decoderReader = NewDecoderReader(nil)
// 	next := func(ctx context.Context) (uint32, *prompb.WriteRequest, error) {
// 		if m.fileHandler == nil {
// 			return 0, nil, io.EOF
// 		}
// 		segID, wr, err := m.decoderReader.Next(ctx, m.fileHandler.Next)
// 		if errors.Is(err, io.EOF) {
// 			if err = m.fileHandler.Remove(); err != nil {
// 				return 0, nil, err
// 			}
// 			m.fileHandler = nil
// 			return 0, nil, nil
// 		}
// 		return segID, wr, err
// 	}
// 	return func(ctx context.Context) (*ResultHandler, error) {
// 		return wrHandler(ctx, next)
// 	}, nil
// }

// // Close - close TCPReader.
// func (m *Manager) Close() error {
// 	if m.fileHandler != nil {
// 		// TODO handler error? file clearing
// 		_ = m.fileHandler.Close()
// 		_ = m.fileHandler.Remove()
// 	}

// 	if m.decoderReader != nil {
// 		m.decoderReader.Shutdown()
// 	}

// 	return m.reader.Close()
// }

// // TCPReader2 - wrappers over connection from cient.
// type TCPReader2 struct {
// 	tt *transport.Transport
// }

// // NewTCPReader2 - init new TCPReader.
// func NewTCPReader2(
// 	cfg *transport.Config,
// 	conn net.Conn,
// ) (*TCPReader2, error) {
// 	return &TCPReader2{
// 		tt: transport.New(cfg, conn),
// 	}, nil
// }

// // Authorization - incoming connection authorization.
// func (r *TCPReader2) Authorization(ctx context.Context) (*transport.AuthMsg, error) {
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
// func (r *TCPReader2) SendResponse(ctx context.Context, text string, code, segmentID uint32) error {
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

// // Next - read from connection and return raw message.
// func (r *TCPReader2) Next(ctx context.Context) (*transport.RawMessage, error) {
// 	return r.tt.Read(ctx)
// }

// // Close - close TCPReader.
// func (r *TCPReader2) Close() error {
// 	return r.tt.Close()
// }

// // FileHandler - reader and writer from file.
// type FileHandler struct {
// 	dir     string
// 	file    *os.File
// 	isClose bool
// }

// // NewFileHandler - init new FileHandler.
// func NewFileHandler(prefixDir, prefixFile string) (*FileHandler, error) {
// 	// make tmp dir for work
// 	dir, err := os.MkdirTemp("", fmt.Sprintf("%s-", prefixDir))
// 	if err != nil {
// 		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
// 	}

// 	// make tmp file for work
// 	file, err := os.CreateTemp(dir, prefixFile)
// 	if err != nil {
// 		_ = os.RemoveAll(dir)
// 		return nil, fmt.Errorf("open file: %w", err)
// 	}

// 	return &FileHandler{
// 		dir:     dir,
// 		file:    file,
// 		isClose: false,
// 	}, nil
// }

// // Write - write RawMessage in file.
// func (fh *FileHandler) Write(raw *transport.RawMessage) error {
// 	return transport.WriteRawMessage(fh.file, raw)
// }

// // Copy - copy data from nextFn to file count times.
// func (fh *FileHandler) Copy(ctx context.Context, count int, next nextRawFn) error {
// 	for i := 0; i < count; i++ {
// 		raw, err := next(ctx)
// 		if err != nil {
// 			return err
// 		}
// 		switch raw.Header.Type {
// 		case transport.MsgSnapshot, transport.MsgDryPut, transport.MsgPut:
// 			if err = fh.Write(raw); err != nil {
// 				return err
// 			}
// 		default:
// 			return UnknownMsgType(int8(raw.Header.Type))
// 		}
// 	}

// 	if err := fh.Sync(); err != nil {
// 		return fmt.Errorf("sync file: %w", err)
// 	}

// 	return nil
// }

// // Sync - implement file sync.
// func (fh *FileHandler) Sync() error {
// 	return fh.file.Sync()
// }

// // SeekStart - to read the file from the beginning.
// func (fh *FileHandler) SeekStart() error {
// 	if _, err := fh.file.Seek(0, io.SeekStart); err != nil {
// 		return err
// 	}

// 	return nil
// }

// // Next - read from file and return raw message.
// func (fh *FileHandler) Next(ctx context.Context) (*transport.RawMessage, error) {
// 	if ctx.Err() != nil {
// 		return nil, ctx.Err()
// 	}

// 	raw, err := transport.ReadRawMessage(fh.file)
// 	if err != nil {
// 		return nil, fmt.Errorf("read file %s: %w", fh.file.Name(), err)
// 	}

// 	return raw, nil
// }

// // WorkDir - return dir for work handler.
// func (fh *FileHandler) WorkDir() string {
// 	return fh.dir
// }

// // Close - close the file handler.
// func (fh *FileHandler) Close() error {
// 	if fh.isClose {
// 		return nil
// 	}

// 	fh.isClose = true
// 	return fh.file.Close()
// }

// // Remove - remove dir and all files.
// func (fh *FileHandler) Remove() error {
// 	_ = fh.Close()

// 	return os.RemoveAll(fh.dir)
// }

// // DecoderReader - message encoded reader.
// type DecoderReader struct {
// 	decoder        *Decoder
// 	startRaw       *transport.RawMessage
// 	decodeSnapshot bool
// 	decodeSegments uint32
// }

// // NewDecoderReader - init new DecoderReader.
// func NewDecoderReader(startRaw *transport.RawMessage) *DecoderReader {
// 	return &DecoderReader{
// 		decoder:        NewDecoder(),
// 		startRaw:       startRaw,
// 		decodeSnapshot: false,
// 		decodeSegments: 0,
// 	}
// }

// // checkForRecreateDecoder - check if there was a snapshot and if it was necessary to recreate the decoder.
// func (dr *DecoderReader) checkForRecreateDecoder() {
// 	if dr.decodeSnapshot || dr.decodeSegments != 0 {
// 		dr.decoder.Destroy()
// 		dr.decoder = NewDecoder()
// 	}

// 	dr.decodeSegments = 0
// 	dr.decodeSnapshot = !dr.decodeSnapshot
// }

// // snapshot - decode snapshot for restore decoder.
// func (dr *DecoderReader) snapshot(ctx context.Context, snapshot []byte) error {
// 	dr.checkForRecreateDecoder()

// 	return dr.decoder.Snapshot(ctx, snapshot)
// }

// // segmentDry - decode segment dry for restore decoder.
// func (dr *DecoderReader) segmentDry(ctx context.Context, segment []byte) error {
// 	dr.decodeSegments++
// 	return dr.decoder.DecodeDry(ctx, segment)
// }

// // segment - decode segment, unmarshal and return id, WriteRequest.
// func (dr *DecoderReader) segment(
// 	ctx context.Context,
// 	segment []byte,
// ) (uint32, *prompb.WriteRequest, error) {
// 	blob, segmentID, err := dr.decoder.Decode(ctx, segment)
// 	if err != nil {
// 		return 0, nil, fmt.Errorf("decode segment: %w", err)
// 	}

// 	msg := new(prompb.WriteRequest)
// 	err = msg.Unmarshal(blob.Bytes())
// 	blob.Destroy()
// 	if err != nil {
// 		return segmentID, nil, fmt.Errorf("unmarshal protobuf: %w", err)
// 	}
// 	dr.decodeSegments++
// 	return segmentID, msg, nil
// }

// // Next - receives a new raw message from the reader and passes it through the decoder, returns a WriteRequest.
// func (dr *DecoderReader) Next(ctx context.Context, next nextRawFn) (uint32, *prompb.WriteRequest, error) {
// 	var (
// 		raw *transport.RawMessage
// 		err error
// 	)

// 	for {
// 		// implementation startWith
// 		if dr.startRaw != nil {
// 			raw = dr.startRaw
// 			dr.startRaw = nil
// 		} else {
// 			raw, err = next(ctx)
// 			if err != nil {
// 				return 0, nil, err
// 			}
// 		}

// 		switch raw.Header.Type {
// 		case transport.MsgSnapshot:
// 			if err = dr.snapshot(ctx, raw.Payload); err != nil {
// 				return 0, nil, fmt.Errorf("decode snapshot: %w", err)
// 			}
// 		case transport.MsgDryPut:
// 			if err = dr.segmentDry(ctx, raw.Payload); err != nil {
// 				return 0, nil, fmt.Errorf("decode segment dry: %w", err)
// 			}
// 		case transport.MsgPut:
// 			return dr.segment(ctx, raw.Payload)
// 		default:
// 			return 0, nil, UnknownMsgType(int8(raw.Header.Type))
// 		}
// 	}
// }

// // Shutdown - shutdown the reader.
// func (dr *DecoderReader) Shutdown() {
// 	dr.decoder.Destroy()
// }
