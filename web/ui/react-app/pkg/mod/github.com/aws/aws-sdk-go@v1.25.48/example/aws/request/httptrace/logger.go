// build example

package main

import (
	"fmt"
	"io"
	"os"
	"time"
)

// RecordTrace outputs the request trace as text.
func RecordTrace(w io.Writer, trace *RequestTrace) {
	attempt := AttemptReport{
		Reused:     trace.Reused,
		Latency:    trace.Finish.Sub(trace.Start),
		ReqWritten: trace.RequestWritten.Sub(trace.Start),
	}

	if !trace.FirstResponseByte.IsZero() {
		attempt.RespFirstByte = trace.FirstResponseByte.Sub(trace.Start)
		attempt.WaitRespFirstByte = trace.FirstResponseByte.Sub(trace.RequestWritten)
	}

	if !trace.Reused {
		attempt.DNSStart = trace.DNSStart.Sub(trace.Start)
		attempt.DNSDone = trace.DNSDone.Sub(trace.Start)
		attempt.DNS = trace.DNSDone.Sub(trace.DNSStart)

		attempt.ConnectStart = trace.ConnectStart.Sub(trace.Start)
		attempt.ConnectDone = trace.ConnectDone.Sub(trace.Start)
		attempt.Connect = trace.ConnectDone.Sub(trace.ConnectStart)

		attempt.TLSHandshakeStart = trace.TLSHandshakeStart.Sub(trace.Start)
		attempt.TLSHandshakeDone = trace.TLSHandshakeDone.Sub(trace.Start)
		attempt.TLSHandshake = trace.TLSHandshakeDone.Sub(trace.TLSHandshakeStart)
	}

	_, err := fmt.Fprintln(w,
		"Latency:",
		attempt.Latency,

		"ConnectionReused:",
		fmt.Sprintf("%t", attempt.Reused),

		"DNSStartAt:",
		fmt.Sprintf("%s", attempt.DNSStart),
		"DNSDoneAt:",
		fmt.Sprintf("%s", attempt.DNSDone),
		"DNSDur:",
		fmt.Sprintf("%s", attempt.DNS),

		"ConnectStartAt:",
		fmt.Sprintf("%s", attempt.ConnectStart),
		"ConnectDoneAt:",
		fmt.Sprintf("%s", attempt.ConnectDone),
		"ConnectDur:",
		fmt.Sprintf("%s", attempt.Connect),

		"TLSStatAt:",
		fmt.Sprintf("%s", attempt.TLSHandshakeStart),
		"TLSDoneAt:",
		fmt.Sprintf("%s", attempt.TLSHandshakeDone),
		"TLSDur:",
		fmt.Sprintf("%s", attempt.TLSHandshake),

		"RequestWritten",
		fmt.Sprintf("%s", attempt.ReqWritten),
		"RespFirstByte:",
		fmt.Sprintf("%s", attempt.RespFirstByte),
		"WaitRespFirstByte:",
		fmt.Sprintf("%s", attempt.WaitRespFirstByte),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write request trace, %v\n", err)
	}
}

// AttemptReport proviedes the structured timing information.
type AttemptReport struct {
	Latency time.Duration
	Reused  bool
	Err     error

	DNSStart time.Duration
	DNSDone  time.Duration
	DNS      time.Duration

	ConnectStart time.Duration
	ConnectDone  time.Duration
	Connect      time.Duration

	TLSHandshakeStart time.Duration
	TLSHandshakeDone  time.Duration
	TLSHandshake      time.Duration

	ReqWritten        time.Duration
	RespFirstByte     time.Duration
	WaitRespFirstByte time.Duration
}
