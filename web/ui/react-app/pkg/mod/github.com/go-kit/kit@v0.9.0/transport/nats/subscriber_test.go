package nats_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/server"
	"github.com/nats-io/nats.go"

	"github.com/go-kit/kit/endpoint"
	natstransport "github.com/go-kit/kit/transport/nats"
)

type TestResponse struct {
	String string `json:"str"`
	Error  string `json:"err"`
}

var natsServer *server.Server

func init() {
	natsServer = server.New(&server.Options{
		Host: "localhost",
		Port: 4222,
	})

	go func() {
		natsServer.Start()
	}()

	if ok := natsServer.ReadyForConnections(2 * time.Second); !ok {
		panic("Failed start of NATS")
	}
}

func newNatsConn(t *testing.T) *nats.Conn {
	// Subscriptions and connections are closed asynchronously, so it's possible
	// that there's still a subscription from an old connection that must be closed
	// before the current test can be run.
	for tries := 20; tries > 0; tries-- {
		if natsServer.NumSubscriptions() == 0 {
			break
		}

		time.Sleep(5 * time.Millisecond)
	}

	if n := natsServer.NumSubscriptions(); n > 0 {
		t.Fatalf("found %d active subscriptions on the server", n)
	}

	nc, err := nats.Connect("nats://"+natsServer.Addr().String(), nats.Name(t.Name()))
	if err != nil {
		t.Fatalf("failed to connect to gnatsd server: %s", err)
	}

	return nc
}

func TestSubscriberBadDecode(t *testing.T) {
	nc := newNatsConn(t)
	defer nc.Close()

	handler := natstransport.NewSubscriber(
		func(context.Context, interface{}) (interface{}, error) { return struct{}{}, nil },
		func(context.Context, *nats.Msg) (interface{}, error) { return struct{}{}, errors.New("dang") },
		func(context.Context, string, *nats.Conn, interface{}) error { return nil },
	)

	resp := testRequest(t, nc, handler)

	if want, have := "dang", resp.Error; want != have {
		t.Errorf("want %s, have %s", want, have)
	}

}

func TestSubscriberBadEndpoint(t *testing.T) {
	nc := newNatsConn(t)
	defer nc.Close()

	handler := natstransport.NewSubscriber(
		func(context.Context, interface{}) (interface{}, error) { return struct{}{}, errors.New("dang") },
		func(context.Context, *nats.Msg) (interface{}, error) { return struct{}{}, nil },
		func(context.Context, string, *nats.Conn, interface{}) error { return nil },
	)

	resp := testRequest(t, nc, handler)

	if want, have := "dang", resp.Error; want != have {
		t.Errorf("want %s, have %s", want, have)
	}
}

func TestSubscriberBadEncode(t *testing.T) {
	nc := newNatsConn(t)
	defer nc.Close()

	handler := natstransport.NewSubscriber(
		func(context.Context, interface{}) (interface{}, error) { return struct{}{}, nil },
		func(context.Context, *nats.Msg) (interface{}, error) { return struct{}{}, nil },
		func(context.Context, string, *nats.Conn, interface{}) error { return errors.New("dang") },
	)

	resp := testRequest(t, nc, handler)

	if want, have := "dang", resp.Error; want != have {
		t.Errorf("want %s, have %s", want, have)
	}
}

func TestSubscriberErrorEncoder(t *testing.T) {
	nc := newNatsConn(t)
	defer nc.Close()

	errTeapot := errors.New("teapot")
	code := func(err error) error {
		if err == errTeapot {
			return err
		}
		return errors.New("dang")
	}
	handler := natstransport.NewSubscriber(
		func(context.Context, interface{}) (interface{}, error) { return struct{}{}, errTeapot },
		func(context.Context, *nats.Msg) (interface{}, error) { return struct{}{}, nil },
		func(context.Context, string, *nats.Conn, interface{}) error { return nil },
		natstransport.SubscriberErrorEncoder(func(_ context.Context, err error, reply string, nc *nats.Conn) {
			var r TestResponse
			r.Error = code(err).Error()

			b, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}

			if err := nc.Publish(reply, b); err != nil {
				t.Fatal(err)
			}
		}),
	)

	resp := testRequest(t, nc, handler)

	if want, have := errTeapot.Error(), resp.Error; want != have {
		t.Errorf("want %s, have %s", want, have)
	}
}

func TestSubscriberHappySubject(t *testing.T) {
	step, response := testSubscriber(t)
	step()
	r := <-response

	var resp TestResponse
	err := json.Unmarshal(r.Data, &resp)
	if err != nil {
		t.Fatal(err)
	}

	if want, have := "", resp.Error; want != have {
		t.Errorf("want %s, have %s (%s)", want, have, r.Data)
	}
}

func TestMultipleSubscriberBefore(t *testing.T) {
	nc := newNatsConn(t)
	defer nc.Close()

	var (
		response = struct{ Body string }{"go eat a fly ugly\n"}
		wg       sync.WaitGroup
		done     = make(chan struct{})
	)
	handler := natstransport.NewSubscriber(
		endpoint.Nop,
		func(context.Context, *nats.Msg) (interface{}, error) {
			return struct{}{}, nil
		},
		func(_ context.Context, reply string, nc *nats.Conn, _ interface{}) error {
			b, err := json.Marshal(response)
			if err != nil {
				return err
			}

			return nc.Publish(reply, b)
		},
		natstransport.SubscriberBefore(func(ctx context.Context, _ *nats.Msg) context.Context {
			ctx = context.WithValue(ctx, "one", 1)

			return ctx
		}),
		natstransport.SubscriberBefore(func(ctx context.Context, _ *nats.Msg) context.Context {
			if _, ok := ctx.Value("one").(int); !ok {
				t.Error("Value was not set properly when multiple ServerBefores are used")
			}

			close(done)
			return ctx
		}),
	)

	sub, err := nc.QueueSubscribe("natstransport.test", "natstransport", handler.ServeMsg(nc))
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := nc.Request("natstransport.test", []byte("test data"), 2*time.Second)
		if err != nil {
			t.Fatal(err)
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for finalizer")
	}

	wg.Wait()
}

func TestMultipleSubscriberAfter(t *testing.T) {
	nc := newNatsConn(t)
	defer nc.Close()

	var (
		response = struct{ Body string }{"go eat a fly ugly\n"}
		wg       sync.WaitGroup
		done     = make(chan struct{})
	)
	handler := natstransport.NewSubscriber(
		endpoint.Nop,
		func(context.Context, *nats.Msg) (interface{}, error) {
			return struct{}{}, nil
		},
		func(_ context.Context, reply string, nc *nats.Conn, _ interface{}) error {
			b, err := json.Marshal(response)
			if err != nil {
				return err
			}

			return nc.Publish(reply, b)
		},
		natstransport.SubscriberAfter(func(ctx context.Context, nc *nats.Conn) context.Context {
			ctx = context.WithValue(ctx, "one", 1)

			return ctx
		}),
		natstransport.SubscriberAfter(func(ctx context.Context, nc *nats.Conn) context.Context {
			if _, ok := ctx.Value("one").(int); !ok {
				t.Error("Value was not set properly when multiple ServerAfters are used")
			}

			close(done)
			return ctx
		}),
	)

	sub, err := nc.QueueSubscribe("natstransport.test", "natstransport", handler.ServeMsg(nc))
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := nc.Request("natstransport.test", []byte("test data"), 2*time.Second)
		if err != nil {
			t.Fatal(err)
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for finalizer")
	}

	wg.Wait()
}

func TestSubscriberFinalizerFunc(t *testing.T) {
	nc := newNatsConn(t)
	defer nc.Close()

	var (
		response = struct{ Body string }{"go eat a fly ugly\n"}
		wg       sync.WaitGroup
		done     = make(chan struct{})
	)
	handler := natstransport.NewSubscriber(
		endpoint.Nop,
		func(context.Context, *nats.Msg) (interface{}, error) {
			return struct{}{}, nil
		},
		func(_ context.Context, reply string, nc *nats.Conn, _ interface{}) error {
			b, err := json.Marshal(response)
			if err != nil {
				return err
			}

			return nc.Publish(reply, b)
		},
		natstransport.SubscriberFinalizer(func(ctx context.Context, _ *nats.Msg) {
			close(done)
		}),
	)

	sub, err := nc.QueueSubscribe("natstransport.test", "natstransport", handler.ServeMsg(nc))
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := nc.Request("natstransport.test", []byte("test data"), 2*time.Second)
		if err != nil {
			t.Fatal(err)
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for finalizer")
	}

	wg.Wait()
}

func TestEncodeJSONResponse(t *testing.T) {
	nc := newNatsConn(t)
	defer nc.Close()

	handler := natstransport.NewSubscriber(
		func(context.Context, interface{}) (interface{}, error) {
			return struct {
				Foo string `json:"foo"`
			}{"bar"}, nil
		},
		func(context.Context, *nats.Msg) (interface{}, error) { return struct{}{}, nil },
		natstransport.EncodeJSONResponse,
	)

	sub, err := nc.QueueSubscribe("natstransport.test", "natstransport", handler.ServeMsg(nc))
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	r, err := nc.Request("natstransport.test", []byte("test data"), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if want, have := `{"foo":"bar"}`, strings.TrimSpace(string(r.Data)); want != have {
		t.Errorf("Body: want %s, have %s", want, have)
	}
}

type responseError struct {
	msg string
}

func (m responseError) Error() string {
	return m.msg
}

func TestErrorEncoder(t *testing.T) {
	nc := newNatsConn(t)
	defer nc.Close()

	errResp := struct {
		Error string `json:"err"`
	}{"oh no"}
	handler := natstransport.NewSubscriber(
		func(context.Context, interface{}) (interface{}, error) {
			return nil, responseError{msg: errResp.Error}
		},
		func(context.Context, *nats.Msg) (interface{}, error) { return struct{}{}, nil },
		natstransport.EncodeJSONResponse,
	)

	sub, err := nc.QueueSubscribe("natstransport.test", "natstransport", handler.ServeMsg(nc))
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	r, err := nc.Request("natstransport.test", []byte("test data"), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	b, err := json.Marshal(errResp)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != string(r.Data) {
		t.Errorf("ErrorEncoder: got: %q, expected: %q", r.Data, b)
	}
}

type noContentResponse struct{}

func TestEncodeNoContent(t *testing.T) {
	nc := newNatsConn(t)
	defer nc.Close()

	handler := natstransport.NewSubscriber(
		func(context.Context, interface{}) (interface{}, error) { return noContentResponse{}, nil },
		func(context.Context, *nats.Msg) (interface{}, error) { return struct{}{}, nil },
		natstransport.EncodeJSONResponse,
	)

	sub, err := nc.QueueSubscribe("natstransport.test", "natstransport", handler.ServeMsg(nc))
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	r, err := nc.Request("natstransport.test", []byte("test data"), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if want, have := `{}`, strings.TrimSpace(string(r.Data)); want != have {
		t.Errorf("Body: want %s, have %s", want, have)
	}
}

func TestNoOpRequestDecoder(t *testing.T) {
	nc := newNatsConn(t)
	defer nc.Close()

	handler := natstransport.NewSubscriber(
		func(ctx context.Context, request interface{}) (interface{}, error) {
			if request != nil {
				t.Error("Expected nil request in endpoint when using NopRequestDecoder")
			}
			return nil, nil
		},
		natstransport.NopRequestDecoder,
		natstransport.EncodeJSONResponse,
	)

	sub, err := nc.QueueSubscribe("natstransport.test", "natstransport", handler.ServeMsg(nc))
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	r, err := nc.Request("natstransport.test", []byte("test data"), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if want, have := `null`, strings.TrimSpace(string(r.Data)); want != have {
		t.Errorf("Body: want %s, have %s", want, have)
	}
}

func testSubscriber(t *testing.T) (step func(), resp <-chan *nats.Msg) {
	var (
		stepch   = make(chan bool)
		endpoint = func(context.Context, interface{}) (interface{}, error) {
			<-stepch
			return struct{}{}, nil
		}
		response = make(chan *nats.Msg)
		handler  = natstransport.NewSubscriber(
			endpoint,
			func(context.Context, *nats.Msg) (interface{}, error) { return struct{}{}, nil },
			natstransport.EncodeJSONResponse,
			natstransport.SubscriberBefore(func(ctx context.Context, msg *nats.Msg) context.Context { return ctx }),
			natstransport.SubscriberAfter(func(ctx context.Context, nc *nats.Conn) context.Context { return ctx }),
		)
	)

	go func() {
		nc := newNatsConn(t)
		defer nc.Close()

		sub, err := nc.QueueSubscribe("natstransport.test", "natstransport", handler.ServeMsg(nc))
		if err != nil {
			t.Fatal(err)
		}
		defer sub.Unsubscribe()

		r, err := nc.Request("natstransport.test", []byte("test data"), 2*time.Second)
		if err != nil {
			t.Fatal(err)
		}

		response <- r
	}()

	return func() { stepch <- true }, response
}

func testRequest(t *testing.T, nc *nats.Conn, handler *natstransport.Subscriber) TestResponse {
	sub, err := nc.QueueSubscribe("natstransport.test", "natstransport", handler.ServeMsg(nc))
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	r, err := nc.Request("natstransport.test", []byte("test data"), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var resp TestResponse
	err = json.Unmarshal(r.Data, &resp)
	if err != nil {
		t.Fatal(err)
	}

	return resp
}
