package sarama

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
)

const (
	expectationTimeout = 500 * time.Millisecond
)

type requestHandlerFunc func(req *request) (res encoder)

// mockBroker is a mock Kafka broker. It consists of a TCP server on a
// kernel-selected localhost port that can accept many connections. It reads
// Kafka requests from that connection and passes them to the user specified
// handler function (see SetHandler) that generates respective responses. If
// the handler has not been explicitly specified then the broker returns
// responses set by the Returns function in the exact order they were provided.
// (if a response has a len of 0, nothing is sent, and the client request will
// timeout in this case).
//
// When running tests with one of these, it is strongly recommended to specify
// a timeout to `go test` so that if the broker hangs waiting for a response,
// the test panics.
//
// It is not necessary to prefix message length or correlation ID to your
// response bytes, the server does that automatically as a convenience.
type mockBroker struct {
	brokerID     int32
	port         int32
	closing      chan none
	stopper      chan none
	expectations chan encoder
	listener     net.Listener
	t            *testing.T
	latency      time.Duration
	handler      requestHandlerFunc
	history      []RequestResponse
	lock         sync.Mutex
}

type RequestResponse struct {
	Request  requestBody
	Response encoder
}

func (b *mockBroker) SetLatency(latency time.Duration) {
	b.latency = latency
}

// SetHandler sets the specified function as the request handler. Whenever
// a mock broker reads a request from the wire it passes the request to the
// function and sends back whatever the handler function returns.
func (b *mockBroker) SetHandler(handler requestHandlerFunc) {
	b.lock.Lock()
	b.handler = handler
	b.lock.Unlock()
}

func (b *mockBroker) SetHandlerByMap(handlerMap map[string]MockResponse) {
	b.SetHandler(func(req *request) (res encoder) {
		reqTypeName := reflect.TypeOf(req.body).Elem().Name()
		mockResponse := handlerMap[reqTypeName]
		if mockResponse == nil {
			return nil
		}
		return mockResponse.For(req.body)
	})
}

func (b *mockBroker) BrokerID() int32 {
	return b.brokerID
}

func (b *mockBroker) History() []RequestResponse {
	b.lock.Lock()
	history := make([]RequestResponse, len(b.history))
	copy(history, b.history)
	b.lock.Unlock()
	return history
}

func (b *mockBroker) Port() int32 {
	return b.port
}

func (b *mockBroker) Addr() string {
	return b.listener.Addr().String()
}

func (b *mockBroker) Close() {
	close(b.expectations)
	if len(b.expectations) > 0 {
		buf := bytes.NewBufferString(fmt.Sprintf("mockbroker/%d: not all expectations were satisfied! Still waiting on:\n", b.BrokerID()))
		for e := range b.expectations {
			_, _ = buf.WriteString(spew.Sdump(e))
		}
		b.t.Error(buf.String())
	}
	close(b.closing)
	<-b.stopper
}

func (b *mockBroker) serverLoop() {
	defer close(b.stopper)
	var err error
	var conn net.Conn

	go func() {
		<-b.closing
		safeClose(b.t, b.listener)
	}()

	wg := &sync.WaitGroup{}
	i := 0
	for conn, err = b.listener.Accept(); err == nil; conn, err = b.listener.Accept() {
		wg.Add(1)
		go b.handleRequests(conn, i, wg)
		i++
	}
	wg.Wait()
	Logger.Printf("*** mockbroker/%d: listener closed, err=%v", b.BrokerID(), err)
}

func (b *mockBroker) handleRequests(conn net.Conn, idx int, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		_ = conn.Close()
	}()
	Logger.Printf("*** mockbroker/%d/%d: connection opened", b.BrokerID(), idx)
	var err error

	abort := make(chan none)
	defer close(abort)
	go func() {
		select {
		case <-b.closing:
			_ = conn.Close()
		case <-abort:
		}
	}()

	resHeader := make([]byte, 8)
	for {
		req, err := decodeRequest(conn)
		if err != nil {
			Logger.Printf("*** mockbroker/%d/%d: invalid request: err=%+v, %+v", b.brokerID, idx, err, spew.Sdump(req))
			b.serverError(err)
			break
		}

		if b.latency > 0 {
			time.Sleep(b.latency)
		}

		b.lock.Lock()
		res := b.handler(req)
		b.history = append(b.history, RequestResponse{req.body, res})
		b.lock.Unlock()

		if res == nil {
			Logger.Printf("*** mockbroker/%d/%d: ignored %v", b.brokerID, idx, spew.Sdump(req))
			continue
		}
		Logger.Printf("*** mockbroker/%d/%d: served %v -> %v", b.brokerID, idx, req, res)

		encodedRes, err := encode(res)
		if err != nil {
			b.serverError(err)
			break
		}
		if len(encodedRes) == 0 {
			continue
		}

		binary.BigEndian.PutUint32(resHeader, uint32(len(encodedRes)+4))
		binary.BigEndian.PutUint32(resHeader[4:], uint32(req.correlationID))
		if _, err = conn.Write(resHeader); err != nil {
			b.serverError(err)
			break
		}
		if _, err = conn.Write(encodedRes); err != nil {
			b.serverError(err)
			break
		}
	}
	Logger.Printf("*** mockbroker/%d/%d: connection closed, err=%v", b.BrokerID(), idx, err)
}

func (b *mockBroker) defaultRequestHandler(req *request) (res encoder) {
	select {
	case res, ok := <-b.expectations:
		if !ok {
			return nil
		}
		return res
	case <-time.After(expectationTimeout):
		return nil
	}
}

func (b *mockBroker) serverError(err error) {
	isConnectionClosedError := false
	if _, ok := err.(*net.OpError); ok {
		isConnectionClosedError = true
	} else if err == io.EOF {
		isConnectionClosedError = true
	} else if err.Error() == "use of closed network connection" {
		isConnectionClosedError = true
	}

	if isConnectionClosedError {
		return
	}

	b.t.Errorf(err.Error())
}

// newMockBroker launches a fake Kafka broker. It takes a *testing.T as provided by the
// test framework and a channel of responses to use.  If an error occurs it is
// simply logged to the *testing.T and the broker exits.
func newMockBroker(t *testing.T, brokerID int32) *mockBroker {
	return newMockBrokerAddr(t, brokerID, "localhost:0")
}

// newMockBrokerAddr behaves like newMockBroker but listens on the address you give
// it rather than just some ephemeral port.
func newMockBrokerAddr(t *testing.T, brokerID int32, addr string) *mockBroker {
	var err error

	broker := &mockBroker{
		closing:      make(chan none),
		stopper:      make(chan none),
		t:            t,
		brokerID:     brokerID,
		expectations: make(chan encoder, 512),
	}
	broker.handler = broker.defaultRequestHandler

	broker.listener, err = net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	Logger.Printf("*** mockbroker/%d listening on %s\n", brokerID, broker.listener.Addr().String())
	_, portStr, err := net.SplitHostPort(broker.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	tmp, err := strconv.ParseInt(portStr, 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	broker.port = int32(tmp)

	go broker.serverLoop()

	return broker
}

func (b *mockBroker) Returns(e encoder) {
	b.expectations <- e
}
