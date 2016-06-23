package frankenstein

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	consul "github.com/hashicorp/consul/api"
	"github.com/prometheus/common/log"
)

const (
	longPollDuration = 10 * time.Second
)

// ConsulClient is a high-level client for Consul, that exposes operations
// such as CAS and Watch which take callbacks.  It also deals with serialisation.
type ConsulClient interface {
	Get(key string, out interface{}) error
	CAS(key string, out interface{}, f CASCallback) error
	WatchPrefix(prefix string, out interface{}, done chan struct{}, f func(string, interface{}) bool)
}

// CASCallback is the type of the callback to CAS.  If err is nil, out must be non-nil.
type CASCallback func(in interface{}) (out interface{}, retry bool, err error)

type kv interface {
	CAS(p *consul.KVPair, q *consul.WriteOptions) (bool, *consul.WriteMeta, error)
	Get(key string, q *consul.QueryOptions) (*consul.KVPair, *consul.QueryMeta, error)
	List(prefix string, q *consul.QueryOptions) (consul.KVPairs, *consul.QueryMeta, error)
}

type consulClient struct {
	kv
}

// NewConsulClient returns a new ConsulClient.
func NewConsulClient(addr string) (ConsulClient, error) {
	client, err := consul.NewClient(&consul.Config{
		Address: addr,
		Scheme:  "http",
	})
	if err != nil {
		return nil, err
	}
	return &consulClient{client.KV()}, nil
}

var (
	queryOptions = &consul.QueryOptions{
		RequireConsistent: true,
	}
	writeOptions = &consul.WriteOptions{}

	// ErrNotFound is returned by ConsulClient.Get.
	ErrNotFound = fmt.Errorf("Not found")
)

// Get and deserialise a JSON value from Consul.
func (c *consulClient) Get(key string, out interface{}) error {
	kvp, _, err := c.kv.Get(key, queryOptions)
	if err != nil {
		return err
	}
	if kvp == nil {
		return ErrNotFound
	}
	if err := json.NewDecoder(bytes.NewReader(kvp.Value)).Decode(out); err != nil {
		return err
	}
	return nil
}

// CAS atomically modifies a value in a callback.
// If value doesn't exist you'll get nil as an argument to your callback.
func (c *consulClient) CAS(key string, out interface{}, f CASCallback) error {
	var (
		index        = uint64(0)
		retries      = 10
		retry        = true
		intermediate interface{}
	)
	for i := 0; i < retries; i++ {
		kvp, _, err := c.kv.Get(key, queryOptions)
		if err != nil {
			log.Errorf("Error getting %s: %v", key, err)
			continue
		}
		if kvp != nil {
			if err := json.NewDecoder(bytes.NewReader(kvp.Value)).Decode(out); err != nil {
				log.Errorf("Error deserialising %s: %v", key, err)
				continue
			}
			// If key doesn't exist, index will be 0.
			index = kvp.ModifyIndex
			intermediate = out
		}

		intermediate, retry, err = f(intermediate)
		if err != nil {
			log.Errorf("Error CASing %s: %v", key, err)
			if !retry {
				return err
			}
			continue
		}

		if intermediate == nil {
			panic("Callback must instantiate value!")
		}

		value := bytes.Buffer{}
		if err := json.NewEncoder(&value).Encode(intermediate); err != nil {
			log.Errorf("Error serialising value for %s: %v", key, err)
			continue
		}
		ok, _, err := c.kv.CAS(&consul.KVPair{
			Key:         key,
			Value:       value.Bytes(),
			ModifyIndex: index,
		}, writeOptions)
		if err != nil {
			log.Errorf("Error CASing %s: %v", key, err)
			continue
		}
		if !ok {
			log.Errorf("Error CASing %s, trying again %d", key, index)
			continue
		}
		return nil
	}
	return fmt.Errorf("failed to CAS %s", key)
}

func (c *consulClient) WatchPrefix(prefix string, out interface{}, done chan struct{}, f func(string, interface{}) bool) {
	index := uint64(0)
	for {
		select {
		case <-done:
			return
		default:
		}

		kvps, meta, err := c.kv.List(prefix, &consul.QueryOptions{
			RequireConsistent: true,
			WaitIndex:         index,
			WaitTime:          longPollDuration,
		})
		if err != nil {
			log.Errorf("Error getting path %s: %v", prefix, err)
			continue
		}
		if index == meta.LastIndex {
			continue
		}
		index = meta.LastIndex

		for _, kvp := range kvps {
			if err := json.NewDecoder(bytes.NewReader(kvp.Value)).Decode(out); err != nil {
				log.Errorf("Error deserialising %s: %v", kvp.Key, err)
				continue
			}
			if !f(kvp.Key, out) {
				return
			}
		}
	}
}
