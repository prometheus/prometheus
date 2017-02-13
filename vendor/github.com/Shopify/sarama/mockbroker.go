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
	"time"

	"github.com/davecgh/go-spew/spew"
)

const (
	expectationTimeout = 500 * time.Millisecond
)

type requestHandlerFunc func(req *request) (res encoder)

// RequestNotifierFunc is invoked when a mock broker processes a request successfully
// and will provides the number of bytes read and written.
type RequestNotifierFunc func(bytesRead, bytesWritten int)

// MockBroker is a mock Kafka broker that is used in unit tests. It is exposed
// to facilitate testing of higher level or specialized consumers and producers
// built on top of Sarama. Note that it does not 'mimic' the Kafka API protocol,
// but rather provides a facility to do that. It takes care of the TCP
// transport, request unmarshaling, response marshaling, and makes it the test
// writer responsibility to program correct according to the Kafka API protocol
// MockBroker behaviour.
//
// MockBroker is implemented as a TCP server listening on a kernel-selected
// localhost port that can accept many connections. It reads Kafka requests
// from that connection and returns responses programmed by the SetHandlerByMap
// function. If a MockBroker receives a request that it has no programmed
// response for, then it returns nothing and the request times out.
//
// A set of MockRequest builders to define mappings used by MockBroker is
// provided by Sarama. But users can develop MockRequests of their own and use
// them along with or instead of the standard ones.
//
// When running tests with MockBroker it is strongly recommended to specify
// a timeout to `go test` so that if the broker hangs waiting for a response,
// the test panics.
//
// It is not necessary to prefix message length or correlation ID to your
// response bytes, the server does that automatically as a convenience.
type MockBroker struct {
	brokerID     int32
	port         int32
	closing      chan none
	stopper      chan none
	expectations chan encoder
	listener     net.Listener
	t            TestReporter
	latency      time.Duration
	handler      requestHandlerFunc
	notifier     RequestNotifierFunc
	history      []RequestResponse
	lock         sync.Mutex
}

// RequestResponse represents a Request/Response pair processed by MockBroker.
type RequestResponse struct {
	Request  protocolBody
	Response encoder
}

// SetLatency makes broker pause for the specified period every time before
// replying.
func (b *MockBroker) SetLatency(latency time.Duration) {
	b.latency = latency
}

// SetHandlerByMap defines mapping of Request types to MockResponses. When a
// request is received by the broker, it looks up the request type in the map
// and uses the found MockResponse instance to generate an appropriate reply.
// If the request type is not found in the map then nothing is sent.
func (b *MockBroker) SetHandlerByMap(handlerMap map[string]MockResponse) {
	b.setHandler(func(req *request) (res encoder) {
		reqTypeName := reflect.TypeOf(req.body).Elem().Name()
		mockResponse := handlerMap[reqTypeName]
		if mockResponse == nil {
			return nil
		}
		return mockResponse.For(req.body)
	})
}

// SetNotifier set a function that will get invoked whenever a request has been
// processed successfully and will provide the number of bytes read and written
func (b *MockBroker) SetNotifier(notifier RequestNotifierFunc) {
	b.lock.Lock()
	b.notifier = notifier
	b.lock.Unlock()
}

// BrokerID returns broker ID assigned to the broker.
func (b *MockBroker) BrokerID() int32 {
	return b.brokerID
}

// History returns a slice of RequestResponse pairs in the order they were
// processed by the broker. Note that in case of multiple connections to the
// broker the order expected by a test can be different from the order recorded
// in the history, unless some synchronization is implemented in the test.
func (b *MockBroker) History() []RequestResponse {
	b.lock.Lock()
	history := make([]RequestResponse, len(b.history))
	copy(history, b.history)
	b.lock.Unlock()
	return history
}

// Port returns the TCP port number the broker is listening for requests on.
func (b *MockBroker) Port() int32 {
	return b.port
}

// Addr returns the broker connection string in the form "<address>:<port>".
func (b *MockBroker) Addr() string {
	return b.listener.Addr().String()
}

// Close terminates the broker blocking until it stops internal goroutines and
// releases all resources.
func (b *MockBroker) Close() {
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

// setHandler sets the specified function as the request handler. Whenever
// a mock broker reads a request from the wire it passes the request to the
// function and sends back whatever the handler function returns.
func (b *MockBroker) setHandler(handler requestHandlerFunc) {
	b.lock.Lock()
	b.handler = handler
	b.lock.Unlock()
}

func (b *MockBroker) serverLoop() {
	defer close(b.stopper)
	var err error
	var conn net.Conn

	go func() {
		<-b.closing
		err := b.listener.Close()
		if err != nil {
			b.t.Error(err)
		}
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

func (b *MockBroker) handleRequests(conn net.Conn, idx int, wg *sync.WaitGroup) {
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
		req, bytesRead, err := decodeRequest(conn)
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

		encodedRes, err := encode(res, nil)
		if err != nil {
			b.serverError(err)
			break
		}
		if len(encodedRes) == 0 {
			b.lock.Lock()
			if b.notifier != nil {
				b.notifier(bytesRead, 0)
			}
			b.lock.Unlock()
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

		b.lock.Lock()
		if b.notifier != nil {
			b.notifier(bytesRead, len(resHeader)+len(encodedRes))
		}
		b.lock.Unlock()
	}
	Logger.Printf("*** mockbroker/%d/%d: connection closed, err=%v", b.BrokerID(), idx, err)
}

func (b *MockBroker) defaultRequestHandler(req *request) (res encoder) {
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

func (b *MockBroker) serverError(err error) {
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

// NewMockBroker launches a fake Kafka broker. It takes a TestReporter as provided by the
// test framework and a channel of responses to use.  If an error occurs it is
// simply logged to the TestReporter and the broker exits.
func NewMockBroker(t TestReporter, brokerID int32) *MockBroker {
	return NewMockBrokerAddr(t, brokerID, "localhost:0")
}

// NewMockBrokerAddr behaves like newMockBroker but listens on the address you give
// it rather than just some ephemeral port.
func NewMockBrokerAddr(t TestReporter, brokerID int32, addr string) *MockBroker {
	var err error

	broker := &MockBroker{
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

func (b *MockBroker) Returns(e encoder) {
	b.expectations <- e
}
