package consul

import (
	"context"
	consul "github.com/hashicorp/consul/api"
	"io"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/sd"
)

var _ sd.Instancer = (*Instancer)(nil) // API check

var consulState = []*consul.ServiceEntry{
	{
		Node: &consul.Node{
			Address: "10.0.0.0",
			Node:    "app00.local",
		},
		Service: &consul.AgentService{
			ID:      "search-api-0",
			Port:    8000,
			Service: "search",
			Tags: []string{
				"api",
				"v1",
			},
		},
	},
	{
		Node: &consul.Node{
			Address: "10.0.0.1",
			Node:    "app01.local",
		},
		Service: &consul.AgentService{
			ID:      "search-api-1",
			Port:    8001,
			Service: "search",
			Tags: []string{
				"api",
				"v2",
			},
		},
	},
	{
		Node: &consul.Node{
			Address: "10.0.0.1",
			Node:    "app01.local",
		},
		Service: &consul.AgentService{
			Address: "10.0.0.10",
			ID:      "search-db-0",
			Port:    9000,
			Service: "search",
			Tags: []string{
				"db",
			},
		},
	},
}

func TestInstancer(t *testing.T) {
	var (
		logger = log.NewNopLogger()
		client = newTestClient(consulState)
	)

	s := NewInstancer(client, logger, "search", []string{"api"}, true)
	defer s.Stop()

	state := s.cache.State()
	if want, have := 2, len(state.Instances); want != have {
		t.Errorf("want %d, have %d", want, have)
	}
}

func TestInstancerNoService(t *testing.T) {
	var (
		logger = log.NewNopLogger()
		client = newTestClient(consulState)
	)

	s := NewInstancer(client, logger, "feed", []string{}, true)
	defer s.Stop()

	state := s.cache.State()
	if want, have := 0, len(state.Instances); want != have {
		t.Fatalf("want %d, have %d", want, have)
	}
}

func TestInstancerWithTags(t *testing.T) {
	var (
		logger = log.NewNopLogger()
		client = newTestClient(consulState)
	)

	s := NewInstancer(client, logger, "search", []string{"api", "v2"}, true)
	defer s.Stop()

	state := s.cache.State()
	if want, have := 1, len(state.Instances); want != have {
		t.Fatalf("want %d, have %d", want, have)
	}
}

func TestInstancerAddressOverride(t *testing.T) {
	s := NewInstancer(newTestClient(consulState), log.NewNopLogger(), "search", []string{"db"}, true)
	defer s.Stop()

	state := s.cache.State()
	if want, have := 1, len(state.Instances); want != have {
		t.Fatalf("want %d, have %d", want, have)
	}

	endpoint, closer, err := testFactory(state.Instances[0])
	if err != nil {
		t.Fatal(err)
	}
	if closer != nil {
		defer closer.Close()
	}

	response, err := endpoint(context.Background(), struct{}{})
	if err != nil {
		t.Fatal(err)
	}

	if want, have := "10.0.0.10:9000", response.(string); want != have {
		t.Errorf("want %q, have %q", want, have)
	}
}

type eofTestClient struct {
	client *testClient
	eofSig chan bool
	called chan struct{}
}

func neweofTestClient(client *testClient, sig chan bool, called chan struct{}) Client {
	return &eofTestClient{client: client, eofSig: sig, called: called}
}

func (c *eofTestClient) Register(r *consul.AgentServiceRegistration) error {
	return c.client.Register(r)
}

func (c *eofTestClient) Deregister(r *consul.AgentServiceRegistration) error {
	return c.client.Deregister(r)
}

func (c *eofTestClient) Service(service, tag string, passingOnly bool, queryOpts *consul.QueryOptions) ([]*consul.ServiceEntry, *consul.QueryMeta, error) {
	c.called <- struct{}{}
	shouldEOF := <-c.eofSig
	if shouldEOF {
		return nil, &consul.QueryMeta{}, io.EOF
	}
	return c.client.Service(service, tag, passingOnly, queryOpts)
}

func TestInstancerWithEOF(t *testing.T) {
	var (
		sig    = make(chan bool, 1)
		called = make(chan struct{}, 1)
		logger = log.NewNopLogger()
		client = neweofTestClient(newTestClient(consulState), sig, called)
	)

	sig <- false
	s := NewInstancer(client, logger, "search", []string{"api"}, true)
	defer s.Stop()

	select {
	case <-called:
	case <-time.Tick(time.Millisecond * 500):
		t.Error("failed, to receive call")
	}

	state := s.cache.State()
	if want, have := 2, len(state.Instances); want != have {
		t.Errorf("want %d, have %d", want, have)
	}

	// some error occurred resulting in io.EOF
	sig <- true

	// Service Called Once
	select {
	case <-called:
	case <-time.Tick(time.Millisecond * 500):
		t.Error("failed, to receive call in time")
	}

	sig <- false

	// loop should continue
	select {
	case <-called:
	case <-time.Tick(time.Millisecond * 500):
		t.Error("failed, to receive call in time")
	}
}
