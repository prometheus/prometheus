package testutil

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"testing"

	"github.com/hashicorp/consul/consul/structs"
	"github.com/pkg/errors"
)

// JoinLAN is used to join local datacenters together.
func (s *TestServer) JoinLAN(t *testing.T, addr string) {
	resp := s.get(t, "/v1/agent/join/"+addr)
	defer resp.Body.Close()
}

// JoinWAN is used to join remote datacenters together.
func (s *TestServer) JoinWAN(t *testing.T, addr string) {
	resp := s.get(t, "/v1/agent/join/"+addr+"?wan=1")
	resp.Body.Close()
}

// SetKV sets an individual key in the K/V store.
func (s *TestServer) SetKV(t *testing.T, key string, val []byte) {
	resp := s.put(t, "/v1/kv/"+key, bytes.NewBuffer(val))
	resp.Body.Close()
}

// SetKVString sets an individual key in the K/V store, but accepts a string
// instead of []byte.
func (s *TestServer) SetKVString(t *testing.T, key string, val string) {
	resp := s.put(t, "/v1/kv/"+key, bytes.NewBufferString(val))
	resp.Body.Close()
}

// GetKV retrieves a single key and returns its value
func (s *TestServer) GetKV(t *testing.T, key string) []byte {
	resp := s.get(t, "/v1/kv/"+key)
	defer resp.Body.Close()

	raw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %s", err)
	}

	var result []*TestKVResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("failed to unmarshal: %s", err)
	}
	if len(result) < 1 {
		t.Fatalf("key does not exist: %s", key)
	}

	v, err := base64.StdEncoding.DecodeString(result[0].Value)
	if err != nil {
		t.Fatalf("failed to base64 decode: %s", err)
	}

	return v
}

// GetKVString retrieves a value from the store, but returns as a string instead
// of []byte.
func (s *TestServer) GetKVString(t *testing.T, key string) string {
	return string(s.GetKV(t, key))
}

// PopulateKV fills the Consul KV with data from a generic map.
func (s *TestServer) PopulateKV(t *testing.T, data map[string][]byte) {
	for k, v := range data {
		s.SetKV(t, k, v)
	}
}

// ListKV returns a list of keys present in the KV store. This will list all
// keys under the given prefix recursively and return them as a slice.
func (s *TestServer) ListKV(t *testing.T, prefix string) []string {
	resp := s.get(t, "/v1/kv/"+prefix+"?keys")
	defer resp.Body.Close()

	raw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %s", err)
	}

	var result []string
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("failed to unmarshal: %s", err)
	}
	return result
}

// AddService adds a new service to the Consul instance. It also
// automatically adds a health check with the given status, which
// can be one of "passing", "warning", or "critical".
func (s *TestServer) AddService(t *testing.T, name, status string, tags []string) {
	svc := &TestService{
		Name: name,
		Tags: tags,
	}
	payload, err := s.encodePayload(svc)
	if err != nil {
		t.Fatal(err)
	}
	s.put(t, "/v1/agent/service/register", payload)

	chkName := "service:" + name
	chk := &TestCheck{
		Name:      chkName,
		ServiceID: name,
		TTL:       "10m",
	}
	payload, err = s.encodePayload(chk)
	if err != nil {
		t.Fatal(err)
	}
	s.put(t, "/v1/agent/check/register", payload)

	switch status {
	case structs.HealthPassing:
		s.put(t, "/v1/agent/check/pass/"+chkName, nil)
	case structs.HealthWarning:
		s.put(t, "/v1/agent/check/warn/"+chkName, nil)
	case structs.HealthCritical:
		s.put(t, "/v1/agent/check/fail/"+chkName, nil)
	default:
		t.Fatalf("Unrecognized status: %s", status)
	}
}

// AddCheck adds a check to the Consul instance. If the serviceID is
// left empty (""), then the check will be associated with the node.
// The check status may be "passing", "warning", or "critical".
func (s *TestServer) AddCheck(t *testing.T, name, serviceID, status string) {
	chk := &TestCheck{
		ID:   name,
		Name: name,
		TTL:  "10m",
	}
	if serviceID != "" {
		chk.ServiceID = serviceID
	}

	payload, err := s.encodePayload(chk)
	if err != nil {
		t.Fatal(err)
	}
	s.put(t, "/v1/agent/check/register", payload)

	switch status {
	case structs.HealthPassing:
		s.put(t, "/v1/agent/check/pass/"+name, nil)
	case structs.HealthWarning:
		s.put(t, "/v1/agent/check/warn/"+name, nil)
	case structs.HealthCritical:
		s.put(t, "/v1/agent/check/fail/"+name, nil)
	default:
		t.Fatalf("Unrecognized status: %s", status)
	}
}

// put performs a new HTTP PUT request.
func (s *TestServer) put(t *testing.T, path string, body io.Reader) *http.Response {
	req, err := http.NewRequest("PUT", s.url(path), body)
	if err != nil {
		t.Fatalf("failed to create PUT request: %s", err)
	}
	resp, err := s.HttpClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make PUT request: %s", err)
	}
	if err := s.requireOK(resp); err != nil {
		defer resp.Body.Close()
		t.Fatalf("not OK PUT: %s", err)
	}
	return resp
}

// get performs a new HTTP GET request.
func (s *TestServer) get(t *testing.T, path string) *http.Response {
	resp, err := s.HttpClient.Get(s.url(path))
	if err != nil {
		t.Fatalf("failed to create GET request: %s", err)
	}
	if err := s.requireOK(resp); err != nil {
		defer resp.Body.Close()
		t.Fatalf("not OK GET: %s", err)
	}
	return resp
}

// encodePayload returns a new io.Reader wrapping the encoded contents
// of the payload, suitable for passing directly to a new request.
func (s *TestServer) encodePayload(payload interface{}) (io.Reader, error) {
	var encoded bytes.Buffer
	enc := json.NewEncoder(&encoded)
	if err := enc.Encode(payload); err != nil {
		return nil, errors.Wrap(err, "failed to encode payload")
	}
	return &encoded, nil
}

// url is a helper function which takes a relative URL and
// makes it into a proper URL against the local Consul server.
func (s *TestServer) url(path string) string {
	if s == nil {
		log.Fatal("s is nil")
	}
	if s.Config == nil {
		log.Fatal("s.Config is nil")
	}
	if s.Config.Ports == nil {
		log.Fatal("s.Config.Ports is nil")
	}
	if s.Config.Ports.HTTP == 0 {
		log.Fatal("s.Config.Ports.HTTP is 0")
	}
	if path == "" {
		log.Fatal("path is empty")
	}
	return fmt.Sprintf("http://127.0.0.1:%d%s", s.Config.Ports.HTTP, path)
}

// requireOK checks the HTTP response code and ensures it is acceptable.
func (s *TestServer) requireOK(resp *http.Response) error {
	if resp.StatusCode != 200 {
		return fmt.Errorf("Bad status code: %d", resp.StatusCode)
	}
	return nil
}
