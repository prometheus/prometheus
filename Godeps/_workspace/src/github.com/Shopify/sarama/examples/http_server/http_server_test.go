package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/Shopify/sarama/mocks"
)

// In normal operation, we expect one access log entry,
// and one data collector entry. Let's assume both will succeed.
// We should return a HTTP 200 status.
func TestCollectSuccessfully(t *testing.T) {
	dataCollectorMock := mocks.NewSyncProducer(t, nil)
	dataCollectorMock.ExpectSendMessageAndSucceed()

	accessLogProducerMock := mocks.NewAsyncProducer(t, nil)
	accessLogProducerMock.ExpectInputAndSucceed()

	// Now, use dependency injection to use the mocks.
	s := &Server{
		DataCollector:     dataCollectorMock,
		AccessLogProducer: accessLogProducerMock,
	}

	// The Server's Close call is important; it will call Close on
	// the two mock producers, which will then validate whether all
	// expectations are resolved.
	defer safeClose(t, s)

	req, err := http.NewRequest("GET", "http://example.com/?data", nil)
	if err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, req)

	if res.Code != 200 {
		t.Errorf("Expected HTTP status 200, found %d", res.Code)
	}

	if string(res.Body.Bytes()) != "Your data is stored with unique identifier important/0/1" {
		t.Error("Unexpected response body", res.Body)
	}
}

// Now, let's see if we handle the case of not being able to produce
// to the data collector properly. In this case we should return a 500 status.
func TestCollectionFailure(t *testing.T) {
	dataCollectorMock := mocks.NewSyncProducer(t, nil)
	dataCollectorMock.ExpectSendMessageAndFail(sarama.ErrRequestTimedOut)

	accessLogProducerMock := mocks.NewAsyncProducer(t, nil)
	accessLogProducerMock.ExpectInputAndSucceed()

	s := &Server{
		DataCollector:     dataCollectorMock,
		AccessLogProducer: accessLogProducerMock,
	}
	defer safeClose(t, s)

	req, err := http.NewRequest("GET", "http://example.com/?data", nil)
	if err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, req)

	if res.Code != 500 {
		t.Errorf("Expected HTTP status 500, found %d", res.Code)
	}
}

// We don't expect any data collector calls because the path is wrong,
// so we are not setting any expectations on the dataCollectorMock. It
// will still generate an access log entry though.
func TestWrongPath(t *testing.T) {
	dataCollectorMock := mocks.NewSyncProducer(t, nil)

	accessLogProducerMock := mocks.NewAsyncProducer(t, nil)
	accessLogProducerMock.ExpectInputAndSucceed()

	s := &Server{
		DataCollector:     dataCollectorMock,
		AccessLogProducer: accessLogProducerMock,
	}
	defer safeClose(t, s)

	req, err := http.NewRequest("GET", "http://example.com/wrong?data", nil)
	if err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()

	s.Handler().ServeHTTP(res, req)

	if res.Code != 404 {
		t.Errorf("Expected HTTP status 404, found %d", res.Code)
	}
}

func safeClose(t *testing.T, o io.Closer) {
	if err := o.Close(); err != nil {
		t.Error(err)
	}
}
