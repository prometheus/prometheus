// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package jsonrpc2 is a minimal implementation of the JSON RPC 2 spec.
// https://www.jsonrpc.org/specification
// It is intended to be compatible with other implementations at the wire level.
package jsonrpc2

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/debug/tag"
	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/telemetry/event"
)

// Conn is a JSON RPC 2 client server connection.
// Conn is bidirectional; it does not have a designated server or client end.
type Conn struct {
	seq        int64 // must only be accessed using atomic operations
	handlers   []Handler
	stream     Stream
	pendingMu  sync.Mutex // protects the pending map
	pending    map[ID]chan *WireResponse
	handlingMu sync.Mutex // protects the handling map
	handling   map[ID]*Request
}

type requestState int

const (
	requestWaiting = requestState(iota)
	requestSerial
	requestParallel
	requestReplied
	requestDone
)

// Request is sent to a server to represent a Call or Notify operaton.
type Request struct {
	conn        *Conn
	cancel      context.CancelFunc
	state       requestState
	nextRequest chan struct{}

	// The Wire values of the request.
	WireRequest
}

// NewErrorf builds a Error struct for the supplied message and code.
// If args is not empty, message and args will be passed to Sprintf.
func NewErrorf(code int64, format string, args ...interface{}) *Error {
	return &Error{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// NewConn creates a new connection object around the supplied stream.
// You must call Run for the connection to be active.
func NewConn(s Stream) *Conn {
	conn := &Conn{
		handlers: []Handler{defaultHandler{}},
		stream:   s,
		pending:  make(map[ID]chan *WireResponse),
		handling: make(map[ID]*Request),
	}
	return conn
}

// AddHandler adds a new handler to the set the connection will invoke.
// Handlers are invoked in the reverse order of how they were added, this
// allows the most recent addition to be the first one to attempt to handle a
// message.
func (c *Conn) AddHandler(handler Handler) {
	// prepend the new handlers so we use them first
	c.handlers = append([]Handler{handler}, c.handlers...)
}

// Cancel cancels a pending Call on the server side.
// The call is identified by its id.
// JSON RPC 2 does not specify a cancel message, so cancellation support is not
// directly wired in. This method allows a higher level protocol to choose how
// to propagate the cancel.
func (c *Conn) Cancel(id ID) {
	c.handlingMu.Lock()
	handling, found := c.handling[id]
	c.handlingMu.Unlock()
	if found {
		handling.cancel()
	}
}

// Notify is called to send a notification request over the connection.
// It will return as soon as the notification has been sent, as no response is
// possible.
func (c *Conn) Notify(ctx context.Context, method string, params interface{}) (err error) {
	jsonParams, err := marshalToRaw(params)
	if err != nil {
		return fmt.Errorf("marshalling notify parameters: %v", err)
	}
	request := &WireRequest{
		Method: method,
		Params: jsonParams,
	}
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshalling notify request: %v", err)
	}
	for _, h := range c.handlers {
		ctx = h.Request(ctx, c, Send, request)
	}
	ctx, done := event.StartSpan(ctx, request.Method,
		tag.Method.Of(request.Method),
		tag.RPCDirection.Of(tag.Outbound),
		tag.RPCID.Of(request.ID.String()),
	)
	defer func() {
		recordStatus(ctx, err)
		done()
	}()

	event.Record(ctx, tag.Started.Of(1))
	n, err := c.stream.Write(ctx, data)
	event.Record(ctx, tag.ReceivedBytes.Of(n))
	return err
}

// Call sends a request over the connection and then waits for a response.
// If the response is not an error, it will be decoded into result.
// result must be of a type you an pass to json.Unmarshal.
func (c *Conn) Call(ctx context.Context, method string, params, result interface{}) (err error) {
	// generate a new request identifier
	id := ID{Number: atomic.AddInt64(&c.seq, 1)}
	jsonParams, err := marshalToRaw(params)
	if err != nil {
		return fmt.Errorf("marshalling call parameters: %v", err)
	}
	request := &WireRequest{
		ID:     &id,
		Method: method,
		Params: jsonParams,
	}
	// marshal the request now it is complete
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshalling call request: %v", err)
	}
	for _, h := range c.handlers {
		ctx = h.Request(ctx, c, Send, request)
	}
	ctx, done := event.StartSpan(ctx, request.Method,
		tag.Method.Of(request.Method),
		tag.RPCDirection.Of(tag.Outbound),
		tag.RPCID.Of(request.ID.String()),
	)
	defer func() {
		recordStatus(ctx, err)
		done()
	}()
	event.Record(ctx, tag.Started.Of(1))
	// We have to add ourselves to the pending map before we send, otherwise we
	// are racing the response. Also add a buffer to rchan, so that if we get a
	// wire response between the time this call is cancelled and id is deleted
	// from c.pending, the send to rchan will not block.
	rchan := make(chan *WireResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = rchan
	c.pendingMu.Unlock()
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()
	// now we are ready to send
	n, err := c.stream.Write(ctx, data)
	event.Record(ctx, tag.ReceivedBytes.Of(n))
	if err != nil {
		// sending failed, we will never get a response, so don't leave it pending
		return err
	}
	// now wait for the response
	select {
	case response := <-rchan:
		// is it an error response?
		if response.Error != nil {
			return response.Error
		}
		if result == nil || response.Result == nil {
			return nil
		}
		if err := json.Unmarshal(*response.Result, result); err != nil {
			return fmt.Errorf("unmarshalling result: %v", err)
		}
		return nil
	case <-ctx.Done():
		// Allow the handler to propagate the cancel.
		cancelled := false
		for _, h := range c.handlers {
			if h.Cancel(ctx, c, id, cancelled) {
				cancelled = true
			}
		}
		return ctx.Err()
	}
}

// Conn returns the connection that created this request.
func (r *Request) Conn() *Conn { return r.conn }

// IsNotify returns true if this request is a notification.
func (r *Request) IsNotify() bool {
	return r.ID == nil
}

// Parallel indicates that the system is now allowed to process other requests
// in parallel with this one.
// It is safe to call any number of times, but must only be called from the
// request handling go routine.
// It is implied by both reply and by the handler returning.
func (r *Request) Parallel() {
	if r.state >= requestParallel {
		return
	}
	r.state = requestParallel
	close(r.nextRequest)
}

// Reply sends a reply to the given request.
// It is an error to call this if request was not a call.
// You must call this exactly once for any given request.
// It should only be called from the handler go routine.
// If err is set then result will be ignored.
// If the request has not yet dropped into parallel mode
// it will be before this function returns.
func (r *Request) Reply(ctx context.Context, result interface{}, err error) error {
	if r.state >= requestReplied {
		return fmt.Errorf("reply invoked more than once")
	}
	if r.IsNotify() {
		return fmt.Errorf("reply not invoked with a valid call: %v, %v", r.Method, r.Params)
	}
	// reply ends the handling phase of a call, so if we are not yet
	// parallel we should be now. The go routine is allowed to continue
	// to do work after replying, which is why it is important to unlock
	// the rpc system at this point.
	r.Parallel()
	r.state = requestReplied

	var raw *json.RawMessage
	if err == nil {
		raw, err = marshalToRaw(result)
	}
	response := &WireResponse{
		Result: raw,
		ID:     r.ID,
	}
	if err != nil {
		if callErr, ok := err.(*Error); ok {
			response.Error = callErr
		} else {
			response.Error = NewErrorf(0, "%s", err)
		}
	}
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}
	n, err := r.conn.stream.Write(ctx, data)
	event.Record(ctx, tag.ReceivedBytes.Of(n))

	if err != nil {
		// TODO(iancottrell): if a stream write fails, we really need to shut down
		// the whole stream
		return err
	}
	return nil
}

func (c *Conn) setHandling(r *Request, active bool) {
	if r.ID == nil {
		return
	}
	r.conn.handlingMu.Lock()
	defer r.conn.handlingMu.Unlock()
	if active {
		r.conn.handling[*r.ID] = r
	} else {
		delete(r.conn.handling, *r.ID)
	}
}

// combined has all the fields of both Request and Response.
// We can decode this and then work out which it is.
type combined struct {
	VersionTag VersionTag       `json:"jsonrpc"`
	ID         *ID              `json:"id,omitempty"`
	Method     string           `json:"method"`
	Params     *json.RawMessage `json:"params,omitempty"`
	Result     *json.RawMessage `json:"result,omitempty"`
	Error      *Error           `json:"error,omitempty"`
}

// Run blocks until the connection is terminated, and returns any error that
// caused the termination.
// It must be called exactly once for each Conn.
// It returns only when the reader is closed or there is an error in the stream.
func (c *Conn) Run(runCtx context.Context) error {
	// we need to make the next request "lock" in an unlocked state to allow
	// the first incoming request to proceed. All later requests are unlocked
	// by the preceding request going to parallel mode.
	nextRequest := make(chan struct{})
	close(nextRequest)
	for {
		// get the data for a message
		data, n, err := c.stream.Read(runCtx)
		if err != nil {
			// The stream failed, we cannot continue. If the client disconnected
			// normally, we should get ErrDisconnected here.
			return err
		}
		// read a combined message
		msg := &combined{}
		if err := json.Unmarshal(data, msg); err != nil {
			// a badly formed message arrived, log it and continue
			// we trust the stream to have isolated the error to just this message
			continue
		}
		// Work out whether this is a request or response.
		switch {
		case msg.Method != "":
			// If method is set it must be a request.
			reqCtx, cancelReq := context.WithCancel(runCtx)
			thisRequest := nextRequest
			nextRequest = make(chan struct{})
			req := &Request{
				conn:        c,
				cancel:      cancelReq,
				nextRequest: nextRequest,
				WireRequest: WireRequest{
					VersionTag: msg.VersionTag,
					Method:     msg.Method,
					Params:     msg.Params,
					ID:         msg.ID,
				},
			}
			for _, h := range c.handlers {
				reqCtx = h.Request(reqCtx, c, Receive, &req.WireRequest)
			}
			reqCtx, done := event.StartSpan(reqCtx, req.WireRequest.Method,
				tag.Method.Of(req.WireRequest.Method),
				tag.RPCDirection.Of(tag.Inbound),
				tag.RPCID.Of(req.WireRequest.ID.String()),
			)
			event.Record(reqCtx,
				tag.Started.Of(1),
				tag.SentBytes.Of(n))
			c.setHandling(req, true)
			_, queueDone := event.StartSpan(reqCtx, "queued")
			go func() {
				<-thisRequest
				queueDone()
				req.state = requestSerial
				defer func() {
					c.setHandling(req, false)
					if !req.IsNotify() && req.state < requestReplied {
						req.Reply(reqCtx, nil, NewErrorf(CodeInternalError, "method %q did not reply", req.Method))
					}
					req.Parallel()
					recordStatus(reqCtx, nil)
					done()
					cancelReq()
				}()
				delivered := false
				for _, h := range c.handlers {
					if h.Deliver(reqCtx, req, delivered) {
						delivered = true
					}
				}
			}()
		case msg.ID != nil:
			// If method is not set, this should be a response, in which case we must
			// have an id to send the response back to the caller.
			c.pendingMu.Lock()
			rchan, ok := c.pending[*msg.ID]
			c.pendingMu.Unlock()
			if ok {
				response := &WireResponse{
					Result: msg.Result,
					Error:  msg.Error,
					ID:     msg.ID,
				}
				rchan <- response
			}
		default:
		}
	}
}

func marshalToRaw(obj interface{}) (*json.RawMessage, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	raw := json.RawMessage(data)
	return &raw, nil
}

func recordStatus(ctx context.Context, err error) {
	if err != nil {
		event.Label(ctx, tag.StatusCode.Of("ERROR"))
	} else {
		event.Label(ctx, tag.StatusCode.Of("OK"))
	}
}
