/*
 *
 * Copyright 2014 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package transport

import (
	"bytes"
	"errors"
	"io"
	"math"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/tap"
)

// ErrIllegalHeaderWrite indicates that setting header is illegal because of
// the stream's state.
var ErrIllegalHeaderWrite = errors.New("transport: the stream is done or WriteHeader was already called")

// http2Server implements the ServerTransport interface with HTTP2.
type http2Server struct {
	ctx         context.Context
	conn        net.Conn
	remoteAddr  net.Addr
	localAddr   net.Addr
	maxStreamID uint32               // max stream ID ever seen
	authInfo    credentials.AuthInfo // auth info about the connection
	inTapHandle tap.ServerInHandle
	// writableChan synchronizes write access to the transport.
	// A writer acquires the write lock by receiving a value on writableChan
	// and releases it by sending on writableChan.
	writableChan chan int
	// shutdownChan is closed when Close is called.
	// Blocking operations should select on shutdownChan to avoid
	// blocking forever after Close.
	shutdownChan chan struct{}
	framer       *framer
	hBuf         *bytes.Buffer  // the buffer for HPACK encoding
	hEnc         *hpack.Encoder // HPACK encoder
	// The max number of concurrent streams.
	maxStreams uint32
	// controlBuf delivers all the control related tasks (e.g., window
	// updates, reset streams, and various settings) to the controller.
	controlBuf *controlBuffer
	fc         *inFlow
	// sendQuotaPool provides flow control to outbound message.
	sendQuotaPool *quotaPool
	stats         stats.Handler
	// Flag to keep track of reading activity on transport.
	// 1 is true and 0 is false.
	activity uint32 // Accessed atomically.
	// Keepalive and max-age parameters for the server.
	kp keepalive.ServerParameters

	// Keepalive enforcement policy.
	kep keepalive.EnforcementPolicy
	// The time instance last ping was received.
	lastPingAt time.Time
	// Number of times the client has violated keepalive ping policy so far.
	pingStrikes uint8
	// Flag to signify that number of ping strikes should be reset to 0.
	// This is set whenever data or header frames are sent.
	// 1 means yes.
	resetPingStrikes  uint32 // Accessed atomically.
	initialWindowSize int32
	bdpEst            *bdpEstimator

	outQuotaVersion uint32

	mu sync.Mutex // guard the following

	// drainChan is initialized when drain(...) is called the first time.
	// After which the server writes out the first GoAway(with ID 2^31-1) frame.
	// Then an independent goroutine will be launched to later send the second GoAway.
	// During this time we don't want to write another first GoAway(with ID 2^31 -1) frame.
	// Thus call to drain(...) will be a no-op if drainChan is already initialized since draining is
	// already underway.
	drainChan     chan struct{}
	state         transportState
	activeStreams map[uint32]*Stream
	// the per-stream outbound flow control window size set by the peer.
	streamSendQuota uint32
	// idle is the time instant when the connection went idle.
	// This is either the begining of the connection or when the number of
	// RPCs go down to 0.
	// When the connection is busy, this value is set to 0.
	idle time.Time
}

// newHTTP2Server constructs a ServerTransport based on HTTP2. ConnectionError is
// returned if something goes wrong.
func newHTTP2Server(conn net.Conn, config *ServerConfig) (_ ServerTransport, err error) {
	framer := newFramer(conn)
	// Send initial settings as connection preface to client.
	var isettings []http2.Setting
	// TODO(zhaoq): Have a better way to signal "no limit" because 0 is
	// permitted in the HTTP2 spec.
	maxStreams := config.MaxStreams
	if maxStreams == 0 {
		maxStreams = math.MaxUint32
	} else {
		isettings = append(isettings, http2.Setting{
			ID:  http2.SettingMaxConcurrentStreams,
			Val: maxStreams,
		})
	}
	dynamicWindow := true
	iwz := int32(initialWindowSize)
	if config.InitialWindowSize >= defaultWindowSize {
		iwz = config.InitialWindowSize
		dynamicWindow = false
	}
	icwz := int32(initialWindowSize)
	if config.InitialConnWindowSize >= defaultWindowSize {
		icwz = config.InitialConnWindowSize
		dynamicWindow = false
	}
	if iwz != defaultWindowSize {
		isettings = append(isettings, http2.Setting{
			ID:  http2.SettingInitialWindowSize,
			Val: uint32(iwz)})
	}
	if err := framer.writeSettings(true, isettings...); err != nil {
		return nil, connectionErrorf(true, err, "transport: %v", err)
	}
	// Adjust the connection flow control window if needed.
	if delta := uint32(icwz - defaultWindowSize); delta > 0 {
		if err := framer.writeWindowUpdate(true, 0, delta); err != nil {
			return nil, connectionErrorf(true, err, "transport: %v", err)
		}
	}
	kp := config.KeepaliveParams
	if kp.MaxConnectionIdle == 0 {
		kp.MaxConnectionIdle = defaultMaxConnectionIdle
	}
	if kp.MaxConnectionAge == 0 {
		kp.MaxConnectionAge = defaultMaxConnectionAge
	}
	// Add a jitter to MaxConnectionAge.
	kp.MaxConnectionAge += getJitter(kp.MaxConnectionAge)
	if kp.MaxConnectionAgeGrace == 0 {
		kp.MaxConnectionAgeGrace = defaultMaxConnectionAgeGrace
	}
	if kp.Time == 0 {
		kp.Time = defaultServerKeepaliveTime
	}
	if kp.Timeout == 0 {
		kp.Timeout = defaultServerKeepaliveTimeout
	}
	kep := config.KeepalivePolicy
	if kep.MinTime == 0 {
		kep.MinTime = defaultKeepalivePolicyMinTime
	}
	var buf bytes.Buffer
	t := &http2Server{
		ctx:               context.Background(),
		conn:              conn,
		remoteAddr:        conn.RemoteAddr(),
		localAddr:         conn.LocalAddr(),
		authInfo:          config.AuthInfo,
		framer:            framer,
		hBuf:              &buf,
		hEnc:              hpack.NewEncoder(&buf),
		maxStreams:        maxStreams,
		inTapHandle:       config.InTapHandle,
		controlBuf:        newControlBuffer(),
		fc:                &inFlow{limit: uint32(icwz)},
		sendQuotaPool:     newQuotaPool(defaultWindowSize),
		state:             reachable,
		writableChan:      make(chan int, 1),
		shutdownChan:      make(chan struct{}),
		activeStreams:     make(map[uint32]*Stream),
		streamSendQuota:   defaultWindowSize,
		stats:             config.StatsHandler,
		kp:                kp,
		idle:              time.Now(),
		kep:               kep,
		initialWindowSize: iwz,
	}
	if dynamicWindow {
		t.bdpEst = &bdpEstimator{
			bdp:               initialWindowSize,
			updateFlowControl: t.updateFlowControl,
		}
	}
	if t.stats != nil {
		t.ctx = t.stats.TagConn(t.ctx, &stats.ConnTagInfo{
			RemoteAddr: t.remoteAddr,
			LocalAddr:  t.localAddr,
		})
		connBegin := &stats.ConnBegin{}
		t.stats.HandleConn(t.ctx, connBegin)
	}
	go t.controller()
	go t.keepalive()
	t.writableChan <- 0
	return t, nil
}

// operateHeader takes action on the decoded headers.
func (t *http2Server) operateHeaders(frame *http2.MetaHeadersFrame, handle func(*Stream), traceCtx func(context.Context, string) context.Context) (close bool) {
	buf := newRecvBuffer()
	s := &Stream{
		id:  frame.Header().StreamID,
		st:  t,
		buf: buf,
		fc:  &inFlow{limit: uint32(t.initialWindowSize)},
	}

	var state decodeState
	for _, hf := range frame.Fields {
		if err := state.processHeaderField(hf); err != nil {
			if se, ok := err.(StreamError); ok {
				t.controlBuf.put(&resetStream{s.id, statusCodeConvTab[se.Code]})
			}
			return
		}
	}

	if frame.StreamEnded() {
		// s is just created by the caller. No lock needed.
		s.state = streamReadDone
	}
	s.recvCompress = state.encoding
	if state.timeoutSet {
		s.ctx, s.cancel = context.WithTimeout(t.ctx, state.timeout)
	} else {
		s.ctx, s.cancel = context.WithCancel(t.ctx)
	}
	pr := &peer.Peer{
		Addr: t.remoteAddr,
	}
	// Attach Auth info if there is any.
	if t.authInfo != nil {
		pr.AuthInfo = t.authInfo
	}
	s.ctx = peer.NewContext(s.ctx, pr)
	// Cache the current stream to the context so that the server application
	// can find out. Required when the server wants to send some metadata
	// back to the client (unary call only).
	s.ctx = newContextWithStream(s.ctx, s)
	// Attach the received metadata to the context.
	if len(state.mdata) > 0 {
		s.ctx = metadata.NewIncomingContext(s.ctx, state.mdata)
	}
	s.trReader = &transportReader{
		reader: &recvBufferReader{
			ctx:  s.ctx,
			recv: s.buf,
		},
		windowHandler: func(n int) {
			t.updateWindow(s, uint32(n))
		},
	}
	s.recvCompress = state.encoding
	s.method = state.method
	if t.inTapHandle != nil {
		var err error
		info := &tap.Info{
			FullMethodName: state.method,
		}
		s.ctx, err = t.inTapHandle(s.ctx, info)
		if err != nil {
			warningf("transport: http2Server.operateHeaders got an error from InTapHandle: %v", err)
			t.controlBuf.put(&resetStream{s.id, http2.ErrCodeRefusedStream})
			return
		}
	}
	t.mu.Lock()
	if t.state != reachable {
		t.mu.Unlock()
		return
	}
	if uint32(len(t.activeStreams)) >= t.maxStreams {
		t.mu.Unlock()
		t.controlBuf.put(&resetStream{s.id, http2.ErrCodeRefusedStream})
		return
	}
	if s.id%2 != 1 || s.id <= t.maxStreamID {
		t.mu.Unlock()
		// illegal gRPC stream id.
		errorf("transport: http2Server.HandleStreams received an illegal stream id: %v", s.id)
		return true
	}
	t.maxStreamID = s.id
	s.sendQuotaPool = newQuotaPool(int(t.streamSendQuota))
	t.activeStreams[s.id] = s
	if len(t.activeStreams) == 1 {
		t.idle = time.Time{}
	}
	t.mu.Unlock()
	s.requestRead = func(n int) {
		t.adjustWindow(s, uint32(n))
	}
	s.ctx = traceCtx(s.ctx, s.method)
	if t.stats != nil {
		s.ctx = t.stats.TagRPC(s.ctx, &stats.RPCTagInfo{FullMethodName: s.method})
		inHeader := &stats.InHeader{
			FullMethod:  s.method,
			RemoteAddr:  t.remoteAddr,
			LocalAddr:   t.localAddr,
			Compression: s.recvCompress,
			WireLength:  int(frame.Header().Length),
		}
		t.stats.HandleRPC(s.ctx, inHeader)
	}
	handle(s)
	return
}

// HandleStreams receives incoming streams using the given handler. This is
// typically run in a separate goroutine.
// traceCtx attaches trace to ctx and returns the new context.
func (t *http2Server) HandleStreams(handle func(*Stream), traceCtx func(context.Context, string) context.Context) {
	// Check the validity of client preface.
	preface := make([]byte, len(clientPreface))
	if _, err := io.ReadFull(t.conn, preface); err != nil {
		// Only log if it isn't a simple tcp accept check (ie: tcp balancer doing open/close socket)
		if err != io.EOF {
			errorf("transport: http2Server.HandleStreams failed to receive the preface from client: %v", err)
		}
		t.Close()
		return
	}
	if !bytes.Equal(preface, clientPreface) {
		errorf("transport: http2Server.HandleStreams received bogus greeting from client: %q", preface)
		t.Close()
		return
	}

	frame, err := t.framer.readFrame()
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		t.Close()
		return
	}
	if err != nil {
		errorf("transport: http2Server.HandleStreams failed to read initial settings frame: %v", err)
		t.Close()
		return
	}
	atomic.StoreUint32(&t.activity, 1)
	sf, ok := frame.(*http2.SettingsFrame)
	if !ok {
		errorf("transport: http2Server.HandleStreams saw invalid preface type %T from client", frame)
		t.Close()
		return
	}
	t.handleSettings(sf)

	for {
		frame, err := t.framer.readFrame()
		atomic.StoreUint32(&t.activity, 1)
		if err != nil {
			if se, ok := err.(http2.StreamError); ok {
				t.mu.Lock()
				s := t.activeStreams[se.StreamID]
				t.mu.Unlock()
				if s != nil {
					t.closeStream(s)
				}
				t.controlBuf.put(&resetStream{se.StreamID, se.Code})
				continue
			}
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				t.Close()
				return
			}
			warningf("transport: http2Server.HandleStreams failed to read frame: %v", err)
			t.Close()
			return
		}
		switch frame := frame.(type) {
		case *http2.MetaHeadersFrame:
			if t.operateHeaders(frame, handle, traceCtx) {
				t.Close()
				break
			}
		case *http2.DataFrame:
			t.handleData(frame)
		case *http2.RSTStreamFrame:
			t.handleRSTStream(frame)
		case *http2.SettingsFrame:
			t.handleSettings(frame)
		case *http2.PingFrame:
			t.handlePing(frame)
		case *http2.WindowUpdateFrame:
			t.handleWindowUpdate(frame)
		case *http2.GoAwayFrame:
			// TODO: Handle GoAway from the client appropriately.
		default:
			errorf("transport: http2Server.HandleStreams found unhandled frame type %v.", frame)
		}
	}
}

func (t *http2Server) getStream(f http2.Frame) (*Stream, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.activeStreams == nil {
		// The transport is closing.
		return nil, false
	}
	s, ok := t.activeStreams[f.Header().StreamID]
	if !ok {
		// The stream is already done.
		return nil, false
	}
	return s, true
}

// adjustWindow sends out extra window update over the initial window size
// of stream if the application is requesting data larger in size than
// the window.
func (t *http2Server) adjustWindow(s *Stream, n uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == streamDone {
		return
	}
	if w := s.fc.maybeAdjust(n); w > 0 {
		if cw := t.fc.resetPendingUpdate(); cw > 0 {
			t.controlBuf.put(&windowUpdate{0, cw, false})
		}
		t.controlBuf.put(&windowUpdate{s.id, w, true})
	}
}

// updateWindow adjusts the inbound quota for the stream and the transport.
// Window updates will deliver to the controller for sending when
// the cumulative quota exceeds the corresponding threshold.
func (t *http2Server) updateWindow(s *Stream, n uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == streamDone {
		return
	}
	if w := s.fc.onRead(n); w > 0 {
		if cw := t.fc.resetPendingUpdate(); cw > 0 {
			t.controlBuf.put(&windowUpdate{0, cw, false})
		}
		t.controlBuf.put(&windowUpdate{s.id, w, true})
	}
}

// updateFlowControl updates the incoming flow control windows
// for the transport and the stream based on the current bdp
// estimation.
func (t *http2Server) updateFlowControl(n uint32) {
	t.mu.Lock()
	for _, s := range t.activeStreams {
		s.fc.newLimit(n)
	}
	t.initialWindowSize = int32(n)
	t.mu.Unlock()
	t.controlBuf.put(&windowUpdate{0, t.fc.newLimit(n), false})
	t.controlBuf.put(&settings{
		ack: false,
		ss: []http2.Setting{
			{
				ID:  http2.SettingInitialWindowSize,
				Val: uint32(n),
			},
		},
	})

}

func (t *http2Server) handleData(f *http2.DataFrame) {
	size := f.Header().Length
	var sendBDPPing bool
	if t.bdpEst != nil {
		sendBDPPing = t.bdpEst.add(uint32(size))
	}
	// Decouple connection's flow control from application's read.
	// An update on connection's flow control should not depend on
	// whether user application has read the data or not. Such a
	// restriction is already imposed on the stream's flow control,
	// and therefore the sender will be blocked anyways.
	// Decoupling the connection flow control will prevent other
	// active(fast) streams from starving in presence of slow or
	// inactive streams.
	//
	// Furthermore, if a bdpPing is being sent out we can piggyback
	// connection's window update for the bytes we just received.
	if sendBDPPing {
		t.controlBuf.put(&windowUpdate{0, uint32(size), false})
		t.controlBuf.put(bdpPing)
	} else {
		if err := t.fc.onData(uint32(size)); err != nil {
			errorf("transport: http2Server %v", err)
			t.Close()
			return
		}
		if w := t.fc.onRead(uint32(size)); w > 0 {
			t.controlBuf.put(&windowUpdate{0, w, true})
		}
	}
	// Select the right stream to dispatch.
	s, ok := t.getStream(f)
	if !ok {
		return
	}
	if size > 0 {
		s.mu.Lock()
		if s.state == streamDone {
			s.mu.Unlock()
			return
		}
		if err := s.fc.onData(uint32(size)); err != nil {
			s.mu.Unlock()
			t.closeStream(s)
			t.controlBuf.put(&resetStream{s.id, http2.ErrCodeFlowControl})
			return
		}
		if f.Header().Flags.Has(http2.FlagDataPadded) {
			if w := s.fc.onRead(uint32(size) - uint32(len(f.Data()))); w > 0 {
				t.controlBuf.put(&windowUpdate{s.id, w, true})
			}
		}
		s.mu.Unlock()
		// TODO(bradfitz, zhaoq): A copy is required here because there is no
		// guarantee f.Data() is consumed before the arrival of next frame.
		// Can this copy be eliminated?
		if len(f.Data()) > 0 {
			data := make([]byte, len(f.Data()))
			copy(data, f.Data())
			s.write(recvMsg{data: data})
		}
	}
	if f.Header().Flags.Has(http2.FlagDataEndStream) {
		// Received the end of stream from the client.
		s.mu.Lock()
		if s.state != streamDone {
			s.state = streamReadDone
		}
		s.mu.Unlock()
		s.write(recvMsg{err: io.EOF})
	}
}

func (t *http2Server) handleRSTStream(f *http2.RSTStreamFrame) {
	s, ok := t.getStream(f)
	if !ok {
		return
	}
	t.closeStream(s)
}

func (t *http2Server) handleSettings(f *http2.SettingsFrame) {
	if f.IsAck() {
		return
	}
	var ss []http2.Setting
	f.ForeachSetting(func(s http2.Setting) error {
		ss = append(ss, s)
		return nil
	})
	// The settings will be applied once the ack is sent.
	t.controlBuf.put(&settings{ack: true, ss: ss})
}

const (
	maxPingStrikes     = 2
	defaultPingTimeout = 2 * time.Hour
)

func (t *http2Server) handlePing(f *http2.PingFrame) {
	if f.IsAck() {
		if f.Data == goAwayPing.data && t.drainChan != nil {
			close(t.drainChan)
			return
		}
		// Maybe it's a BDP ping.
		if t.bdpEst != nil {
			t.bdpEst.calculate(f.Data)
		}
		return
	}
	pingAck := &ping{ack: true}
	copy(pingAck.data[:], f.Data[:])
	t.controlBuf.put(pingAck)

	now := time.Now()
	defer func() {
		t.lastPingAt = now
	}()
	// A reset ping strikes means that we don't need to check for policy
	// violation for this ping and the pingStrikes counter should be set
	// to 0.
	if atomic.CompareAndSwapUint32(&t.resetPingStrikes, 1, 0) {
		t.pingStrikes = 0
		return
	}
	t.mu.Lock()
	ns := len(t.activeStreams)
	t.mu.Unlock()
	if ns < 1 && !t.kep.PermitWithoutStream {
		// Keepalive shouldn't be active thus, this new ping should
		// have come after atleast defaultPingTimeout.
		if t.lastPingAt.Add(defaultPingTimeout).After(now) {
			t.pingStrikes++
		}
	} else {
		// Check if keepalive policy is respected.
		if t.lastPingAt.Add(t.kep.MinTime).After(now) {
			t.pingStrikes++
		}
	}

	if t.pingStrikes > maxPingStrikes {
		// Send goaway and close the connection.
		t.controlBuf.put(&goAway{code: http2.ErrCodeEnhanceYourCalm, debugData: []byte("too_many_pings"), closeConn: true})
	}
}

func (t *http2Server) handleWindowUpdate(f *http2.WindowUpdateFrame) {
	id := f.Header().StreamID
	incr := f.Increment
	if id == 0 {
		t.sendQuotaPool.add(int(incr))
		return
	}
	if s, ok := t.getStream(f); ok {
		s.sendQuotaPool.add(int(incr))
	}
}

func (t *http2Server) writeHeaders(s *Stream, b *bytes.Buffer, endStream bool) error {
	first := true
	endHeaders := false
	var err error
	defer func() {
		if err == nil {
			// Reset ping strikes when seding headers since that might cause the
			// peer to send ping.
			atomic.StoreUint32(&t.resetPingStrikes, 1)
		}
	}()
	// Sends the headers in a single batch.
	for !endHeaders {
		size := t.hBuf.Len()
		if size > http2MaxFrameLen {
			size = http2MaxFrameLen
		} else {
			endHeaders = true
		}
		if first {
			p := http2.HeadersFrameParam{
				StreamID:      s.id,
				BlockFragment: b.Next(size),
				EndStream:     endStream,
				EndHeaders:    endHeaders,
			}
			err = t.framer.writeHeaders(endHeaders, p)
			first = false
		} else {
			err = t.framer.writeContinuation(endHeaders, s.id, endHeaders, b.Next(size))
		}
		if err != nil {
			t.Close()
			return connectionErrorf(true, err, "transport: %v", err)
		}
	}
	return nil
}

// WriteHeader sends the header metedata md back to the client.
func (t *http2Server) WriteHeader(s *Stream, md metadata.MD) error {
	s.mu.Lock()
	if s.headerOk || s.state == streamDone {
		s.mu.Unlock()
		return ErrIllegalHeaderWrite
	}
	s.headerOk = true
	if md.Len() > 0 {
		if s.header.Len() > 0 {
			s.header = metadata.Join(s.header, md)
		} else {
			s.header = md
		}
	}
	md = s.header
	s.mu.Unlock()
	if _, err := wait(s.ctx, nil, nil, t.shutdownChan, t.writableChan); err != nil {
		return err
	}
	t.hBuf.Reset()
	t.hEnc.WriteField(hpack.HeaderField{Name: ":status", Value: "200"})
	t.hEnc.WriteField(hpack.HeaderField{Name: "content-type", Value: "application/grpc"})
	if s.sendCompress != "" {
		t.hEnc.WriteField(hpack.HeaderField{Name: "grpc-encoding", Value: s.sendCompress})
	}
	for k, vv := range md {
		if isReservedHeader(k) {
			// Clients don't tolerate reading restricted headers after some non restricted ones were sent.
			continue
		}
		for _, v := range vv {
			t.hEnc.WriteField(hpack.HeaderField{Name: k, Value: encodeMetadataHeader(k, v)})
		}
	}
	bufLen := t.hBuf.Len()
	if err := t.writeHeaders(s, t.hBuf, false); err != nil {
		return err
	}
	if t.stats != nil {
		outHeader := &stats.OutHeader{
			WireLength: bufLen,
		}
		t.stats.HandleRPC(s.Context(), outHeader)
	}
	t.writableChan <- 0
	return nil
}

// WriteStatus sends stream status to the client and terminates the stream.
// There is no further I/O operations being able to perform on this stream.
// TODO(zhaoq): Now it indicates the end of entire stream. Revisit if early
// OK is adopted.
func (t *http2Server) WriteStatus(s *Stream, st *status.Status) error {
	var headersSent, hasHeader bool
	s.mu.Lock()
	if s.state == streamDone {
		s.mu.Unlock()
		return nil
	}
	if s.headerOk {
		headersSent = true
	}
	if s.header.Len() > 0 {
		hasHeader = true
	}
	s.mu.Unlock()

	if !headersSent && hasHeader {
		t.WriteHeader(s, nil)
		headersSent = true
	}

	if _, err := wait(s.ctx, nil, nil, t.shutdownChan, t.writableChan); err != nil {
		return err
	}
	t.hBuf.Reset()
	if !headersSent {
		t.hEnc.WriteField(hpack.HeaderField{Name: ":status", Value: "200"})
		t.hEnc.WriteField(hpack.HeaderField{Name: "content-type", Value: "application/grpc"})
	}
	t.hEnc.WriteField(
		hpack.HeaderField{
			Name:  "grpc-status",
			Value: strconv.Itoa(int(st.Code())),
		})
	t.hEnc.WriteField(hpack.HeaderField{Name: "grpc-message", Value: encodeGrpcMessage(st.Message())})

	if p := st.Proto(); p != nil && len(p.Details) > 0 {
		stBytes, err := proto.Marshal(p)
		if err != nil {
			// TODO: return error instead, when callers are able to handle it.
			panic(err)
		}

		t.hEnc.WriteField(hpack.HeaderField{Name: "grpc-status-details-bin", Value: encodeBinHeader(stBytes)})
	}

	// Attach the trailer metadata.
	for k, vv := range s.trailer {
		// Clients don't tolerate reading restricted headers after some non restricted ones were sent.
		if isReservedHeader(k) {
			continue
		}
		for _, v := range vv {
			t.hEnc.WriteField(hpack.HeaderField{Name: k, Value: encodeMetadataHeader(k, v)})
		}
	}
	bufLen := t.hBuf.Len()
	if err := t.writeHeaders(s, t.hBuf, true); err != nil {
		t.Close()
		return err
	}
	if t.stats != nil {
		outTrailer := &stats.OutTrailer{
			WireLength: bufLen,
		}
		t.stats.HandleRPC(s.Context(), outTrailer)
	}
	t.closeStream(s)
	t.writableChan <- 0
	return nil
}

// Write converts the data into HTTP2 data frame and sends it out. Non-nil error
// is returns if it fails (e.g., framing error, transport error).
func (t *http2Server) Write(s *Stream, data []byte, opts *Options) (err error) {
	// TODO(zhaoq): Support multi-writers for a single stream.
	var writeHeaderFrame bool
	s.mu.Lock()
	if s.state == streamDone {
		s.mu.Unlock()
		return streamErrorf(codes.Unknown, "the stream has been done")
	}
	if !s.headerOk {
		writeHeaderFrame = true
	}
	s.mu.Unlock()
	if writeHeaderFrame {
		t.WriteHeader(s, nil)
	}
	r := bytes.NewBuffer(data)
	var (
		p   []byte
		oqv uint32
	)
	for {
		if r.Len() == 0 && p == nil {
			return nil
		}
		oqv = atomic.LoadUint32(&t.outQuotaVersion)
		size := http2MaxFrameLen
		// Wait until the stream has some quota to send the data.
		sq, err := wait(s.ctx, nil, nil, t.shutdownChan, s.sendQuotaPool.acquire())
		if err != nil {
			return err
		}
		// Wait until the transport has some quota to send the data.
		tq, err := wait(s.ctx, nil, nil, t.shutdownChan, t.sendQuotaPool.acquire())
		if err != nil {
			return err
		}
		if sq < size {
			size = sq
		}
		if tq < size {
			size = tq
		}
		if p == nil {
			p = r.Next(size)
		}
		ps := len(p)
		if ps < sq {
			// Overbooked stream quota. Return it back.
			s.sendQuotaPool.add(sq - ps)
		}
		if ps < tq {
			// Overbooked transport quota. Return it back.
			t.sendQuotaPool.add(tq - ps)
		}
		t.framer.adjustNumWriters(1)
		// Got some quota. Try to acquire writing privilege on the
		// transport.
		if _, err := wait(s.ctx, nil, nil, t.shutdownChan, t.writableChan); err != nil {
			if _, ok := err.(StreamError); ok {
				// Return the connection quota back.
				t.sendQuotaPool.add(ps)
			}
			if t.framer.adjustNumWriters(-1) == 0 {
				// This writer is the last one in this batch and has the
				// responsibility to flush the buffered frames. It queues
				// a flush request to controlBuf instead of flushing directly
				// in order to avoid the race with other writing or flushing.
				t.controlBuf.put(&flushIO{})
			}
			return err
		}
		select {
		case <-s.ctx.Done():
			t.sendQuotaPool.add(ps)
			if t.framer.adjustNumWriters(-1) == 0 {
				t.controlBuf.put(&flushIO{})
			}
			t.writableChan <- 0
			return ContextErr(s.ctx.Err())
		default:
		}
		if oqv != atomic.LoadUint32(&t.outQuotaVersion) {
			// InitialWindowSize settings frame must have been received after we
			// acquired send quota but before we got the writable channel.
			// We must forsake this write.
			t.sendQuotaPool.add(ps)
			s.sendQuotaPool.add(ps)
			if t.framer.adjustNumWriters(-1) == 0 {
				t.controlBuf.put(&flushIO{})
			}
			t.writableChan <- 0
			continue
		}
		var forceFlush bool
		if r.Len() == 0 && t.framer.adjustNumWriters(0) == 1 && !opts.Last {
			forceFlush = true
		}
		// Reset ping strikes when sending data since this might cause
		// the peer to send ping.
		atomic.StoreUint32(&t.resetPingStrikes, 1)
		if err := t.framer.writeData(forceFlush, s.id, false, p); err != nil {
			t.Close()
			return connectionErrorf(true, err, "transport: %v", err)
		}
		p = nil
		if t.framer.adjustNumWriters(-1) == 0 {
			t.framer.flushWrite()
		}
		t.writableChan <- 0
	}

}

func (t *http2Server) applySettings(ss []http2.Setting) {
	for _, s := range ss {
		if s.ID == http2.SettingInitialWindowSize {
			t.mu.Lock()
			defer t.mu.Unlock()
			for _, stream := range t.activeStreams {
				stream.sendQuotaPool.add(int(s.Val) - int(t.streamSendQuota))
			}
			t.streamSendQuota = s.Val
			atomic.AddUint32(&t.outQuotaVersion, 1)
		}

	}
}

// keepalive running in a separate goroutine does the following:
// 1. Gracefully closes an idle connection after a duration of keepalive.MaxConnectionIdle.
// 2. Gracefully closes any connection after a duration of keepalive.MaxConnectionAge.
// 3. Forcibly closes a connection after an additive period of keepalive.MaxConnectionAgeGrace over keepalive.MaxConnectionAge.
// 4. Makes sure a connection is alive by sending pings with a frequency of keepalive.Time and closes a non-responsive connection
// after an additional duration of keepalive.Timeout.
func (t *http2Server) keepalive() {
	p := &ping{}
	var pingSent bool
	maxIdle := time.NewTimer(t.kp.MaxConnectionIdle)
	maxAge := time.NewTimer(t.kp.MaxConnectionAge)
	keepalive := time.NewTimer(t.kp.Time)
	// NOTE: All exit paths of this function should reset their
	// respecitve timers. A failure to do so will cause the
	// following clean-up to deadlock and eventually leak.
	defer func() {
		if !maxIdle.Stop() {
			<-maxIdle.C
		}
		if !maxAge.Stop() {
			<-maxAge.C
		}
		if !keepalive.Stop() {
			<-keepalive.C
		}
	}()
	for {
		select {
		case <-maxIdle.C:
			t.mu.Lock()
			idle := t.idle
			if idle.IsZero() { // The connection is non-idle.
				t.mu.Unlock()
				maxIdle.Reset(t.kp.MaxConnectionIdle)
				continue
			}
			val := t.kp.MaxConnectionIdle - time.Since(idle)
			t.mu.Unlock()
			if val <= 0 {
				// The connection has been idle for a duration of keepalive.MaxConnectionIdle or more.
				// Gracefully close the connection.
				t.drain(http2.ErrCodeNo, []byte{})
				// Reseting the timer so that the clean-up doesn't deadlock.
				maxIdle.Reset(infinity)
				return
			}
			maxIdle.Reset(val)
		case <-maxAge.C:
			t.drain(http2.ErrCodeNo, []byte{})
			maxAge.Reset(t.kp.MaxConnectionAgeGrace)
			select {
			case <-maxAge.C:
				// Close the connection after grace period.
				t.Close()
				// Reseting the timer so that the clean-up doesn't deadlock.
				maxAge.Reset(infinity)
			case <-t.shutdownChan:
			}
			return
		case <-keepalive.C:
			if atomic.CompareAndSwapUint32(&t.activity, 1, 0) {
				pingSent = false
				keepalive.Reset(t.kp.Time)
				continue
			}
			if pingSent {
				t.Close()
				// Reseting the timer so that the clean-up doesn't deadlock.
				keepalive.Reset(infinity)
				return
			}
			pingSent = true
			t.controlBuf.put(p)
			keepalive.Reset(t.kp.Timeout)
		case <-t.shutdownChan:
			return
		}
	}
}

var goAwayPing = &ping{data: [8]byte{1, 6, 1, 8, 0, 3, 3, 9}}

// controller running in a separate goroutine takes charge of sending control
// frames (e.g., window update, reset stream, setting, etc.) to the server.
func (t *http2Server) controller() {
	for {
		select {
		case i := <-t.controlBuf.get():
			t.controlBuf.load()
			select {
			case <-t.writableChan:
				switch i := i.(type) {
				case *windowUpdate:
					t.framer.writeWindowUpdate(i.flush, i.streamID, i.increment)
				case *settings:
					if i.ack {
						t.framer.writeSettingsAck(true)
						t.applySettings(i.ss)
					} else {
						t.framer.writeSettings(true, i.ss...)
					}
				case *resetStream:
					t.framer.writeRSTStream(true, i.streamID, i.code)
				case *goAway:
					t.mu.Lock()
					if t.state == closing {
						t.mu.Unlock()
						// The transport is closing.
						return
					}
					sid := t.maxStreamID
					if !i.headsUp {
						// Stop accepting more streams now.
						t.state = draining
						t.mu.Unlock()
						t.framer.writeGoAway(true, sid, i.code, i.debugData)
						if i.closeConn {
							// Abruptly close the connection following the GoAway.
							t.Close()
						}
						t.writableChan <- 0
						continue
					}
					t.mu.Unlock()
					// For a graceful close, send out a GoAway with stream ID of MaxUInt32,
					// Follow that with a ping and wait for the ack to come back or a timer
					// to expire. During this time accept new streams since they might have
					// originated before the GoAway reaches the client.
					// After getting the ack or timer expiration send out another GoAway this
					// time with an ID of the max stream server intends to process.
					t.framer.writeGoAway(true, math.MaxUint32, http2.ErrCodeNo, []byte{})
					t.framer.writePing(true, false, goAwayPing.data)
					go func() {
						timer := time.NewTimer(time.Minute)
						defer timer.Stop()
						select {
						case <-t.drainChan:
						case <-timer.C:
						case <-t.shutdownChan:
							return
						}
						t.controlBuf.put(&goAway{code: i.code, debugData: i.debugData})
					}()
				case *flushIO:
					t.framer.flushWrite()
				case *ping:
					if !i.ack {
						t.bdpEst.timesnap(i.data)
					}
					t.framer.writePing(true, i.ack, i.data)
				default:
					errorf("transport: http2Server.controller got unexpected item type %v\n", i)
				}
				t.writableChan <- 0
				continue
			case <-t.shutdownChan:
				return
			}
		case <-t.shutdownChan:
			return
		}
	}
}

// Close starts shutting down the http2Server transport.
// TODO(zhaoq): Now the destruction is not blocked on any pending streams. This
// could cause some resource issue. Revisit this later.
func (t *http2Server) Close() (err error) {
	t.mu.Lock()
	if t.state == closing {
		t.mu.Unlock()
		return errors.New("transport: Close() was already called")
	}
	t.state = closing
	streams := t.activeStreams
	t.activeStreams = nil
	t.mu.Unlock()
	close(t.shutdownChan)
	err = t.conn.Close()
	// Cancel all active streams.
	for _, s := range streams {
		s.cancel()
	}
	if t.stats != nil {
		connEnd := &stats.ConnEnd{}
		t.stats.HandleConn(t.ctx, connEnd)
	}
	return
}

// closeStream clears the footprint of a stream when the stream is not needed
// any more.
func (t *http2Server) closeStream(s *Stream) {
	t.mu.Lock()
	delete(t.activeStreams, s.id)
	if len(t.activeStreams) == 0 {
		t.idle = time.Now()
	}
	if t.state == draining && len(t.activeStreams) == 0 {
		defer t.Close()
	}
	t.mu.Unlock()
	// In case stream sending and receiving are invoked in separate
	// goroutines (e.g., bi-directional streaming), cancel needs to be
	// called to interrupt the potential blocking on other goroutines.
	s.cancel()
	s.mu.Lock()
	if s.state == streamDone {
		s.mu.Unlock()
		return
	}
	s.state = streamDone
	s.mu.Unlock()
}

func (t *http2Server) RemoteAddr() net.Addr {
	return t.remoteAddr
}

func (t *http2Server) Drain() {
	t.drain(http2.ErrCodeNo, []byte{})
}

func (t *http2Server) drain(code http2.ErrCode, debugData []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.drainChan != nil {
		return
	}
	t.drainChan = make(chan struct{})
	t.controlBuf.put(&goAway{code: code, debugData: debugData, headsUp: true})
}

var rgen = rand.New(rand.NewSource(time.Now().UnixNano()))

func getJitter(v time.Duration) time.Duration {
	if v == infinity {
		return 0
	}
	// Generate a jitter between +/- 10% of the value.
	r := int64(v / 10)
	j := rgen.Int63n(2*r) - r
	return time.Duration(j)
}
