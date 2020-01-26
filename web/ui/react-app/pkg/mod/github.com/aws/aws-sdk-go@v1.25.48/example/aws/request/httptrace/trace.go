package main

import (
	"context"
	"crypto/tls"
	"net/http/httptrace"
	"time"
)

// RequestTrace provides the trace time stamps of the HTTP request's segments.
type RequestTrace struct {
	context.Context

	Start, Finish time.Time

	Reused bool

	DNSStart, DNSDone                   time.Time
	ConnectStart, ConnectDone           time.Time
	TLSHandshakeStart, TLSHandshakeDone time.Time
	RequestWritten                      time.Time
	FirstResponseByte                   time.Time
}

// NewRequestTrace returns a initialized RequestTrace for an
// httptrace.ClientTrace, based on the context passed.
func NewRequestTrace(ctx context.Context) *RequestTrace {
	rt := &RequestTrace{
		Start: time.Now(),
	}

	trace := &httptrace.ClientTrace{
		GetConn:              rt.getConn,
		GotConn:              rt.gotConn,
		PutIdleConn:          rt.putIdleConn,
		GotFirstResponseByte: rt.gotFirstResponseByte,
		Got100Continue:       rt.got100Continue,
		DNSStart:             rt.dnsStart,
		DNSDone:              rt.dnsDone,
		ConnectStart:         rt.connectStart,
		ConnectDone:          rt.connectDone,
		TLSHandshakeStart:    rt.tlsHandshakeStart,
		TLSHandshakeDone:     rt.tlsHandshakeDone,
		WroteHeaders:         rt.wroteHeaders,
		Wait100Continue:      rt.wait100Continue,
		WroteRequest:         rt.wroteRequest,
	}

	rt.Context = httptrace.WithClientTrace(ctx, trace)

	return rt
}

// TotalLatency returns the total time the request took.
func (rt *RequestTrace) TotalLatency() time.Duration {
	return rt.Finish.Sub(rt.Start)
}

// RequestDone completes the request trace.
func (rt *RequestTrace) RequestDone() {
	rt.Finish = time.Now()
}

func (rt *RequestTrace) getConn(hostPort string) {}
func (rt *RequestTrace) gotConn(info httptrace.GotConnInfo) {
	rt.Reused = info.Reused
}
func (rt *RequestTrace) putIdleConn(err error) {}
func (rt *RequestTrace) gotFirstResponseByte() {
	rt.FirstResponseByte = time.Now()
}
func (rt *RequestTrace) got100Continue() {}
func (rt *RequestTrace) dnsStart(info httptrace.DNSStartInfo) {
	rt.DNSStart = time.Now()
}
func (rt *RequestTrace) dnsDone(info httptrace.DNSDoneInfo) {
	rt.DNSDone = time.Now()
}
func (rt *RequestTrace) connectStart(network, addr string) {
	rt.ConnectStart = time.Now()
}
func (rt *RequestTrace) connectDone(network, addr string, err error) {
	rt.ConnectDone = time.Now()
}
func (rt *RequestTrace) tlsHandshakeStart() {
	rt.TLSHandshakeStart = time.Now()
}
func (rt *RequestTrace) tlsHandshakeDone(state tls.ConnectionState, err error) {
	rt.TLSHandshakeDone = time.Now()
}
func (rt *RequestTrace) wroteHeaders()    {}
func (rt *RequestTrace) wait100Continue() {}
func (rt *RequestTrace) wroteRequest(info httptrace.WroteRequestInfo) {
	rt.RequestWritten = time.Now()
}
