// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package frankenstein

import (
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/prometheus/frankenstein/wire"
)

var (
	memcacheRequests = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "prometheus",
		Name:      "memcache_requests_total",
		Help:      "Total count of chunks requested from memcache.",
	})

	memcacheHits = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "prometheus",
		Name:      "memcache_hits_total",
		Help:      "Total count of chunks found in memcache.",
	})

	memcacheRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "prometheus",
		Name:      "memcache_request_duration_seconds",
		Help:      "Total time spent in seconds doing memcache requests.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "status_code"})
)

func init() {
	prometheus.MustRegister(memcacheRequests)
	prometheus.MustRegister(memcacheHits)
	prometheus.MustRegister(memcacheRequestDuration)
}

// MemcacheClient is a memcache client that gets its server list from SRV
// records, and periodically updates that ServerList.
type MemcacheClient struct {
	client     *memcache.Client
	serverList *memcache.ServerList
	expiration int32
	hostname   string
	service    string

	quit chan struct{}
	wait sync.WaitGroup
}

// MemcacheConfig defines how a MemcacheClient should be constructed.
type MemcacheConfig struct {
	Host           string
	Service        string
	Timeout        time.Duration
	UpdateInterval time.Duration
	Expiration     time.Duration
}

// NewMemcacheClient creates a new MemcacheClient that gets its server list
// from SRV and updates the server list on a regular basis.
func NewMemcacheClient(config MemcacheConfig) *MemcacheClient {
	var servers memcache.ServerList
	client := memcache.NewFromSelector(&servers)
	client.Timeout = config.Timeout

	newClient := &MemcacheClient{
		client:     client,
		serverList: &servers,
		expiration: int32(config.Expiration.Seconds()),
		hostname:   config.Host,
		service:    config.Service,
		quit:       make(chan struct{}),
	}
	err := newClient.updateMemcacheServers()
	if err != nil {
		log.Errorf("Error setting memcache servers to '%v': %v", config.Host, err)
	}

	newClient.wait.Add(1)
	go newClient.updateLoop(config.UpdateInterval)
	return newClient
}

// Stop the memcache client.
func (c *MemcacheClient) Stop() {
	close(c.quit)
	c.wait.Wait()
}

func (c *MemcacheClient) updateLoop(updateInterval time.Duration) error {
	defer c.wait.Done()
	ticker := time.NewTicker(updateInterval)
	var err error
	for {
		select {
		case <-ticker.C:
			err = c.updateMemcacheServers()
			if err != nil {
				log.Warnf("Error updating memcache servers: %v", err)
			}
		case <-c.quit:
			ticker.Stop()
		}
	}
}

// updateMemcacheServers sets a memcache server list from SRV records. SRV
// priority & weight are ignored.
func (c *MemcacheClient) updateMemcacheServers() error {
	_, addrs, err := net.LookupSRV(c.service, "tcp", c.hostname)
	if err != nil {
		return err
	}
	var servers []string
	for _, srv := range addrs {
		servers = append(servers, fmt.Sprintf("%s:%d", srv.Target, srv.Port))
	}
	// ServerList deterministically maps keys to _index_ of the server list.
	// Since DNS returns records in different order each time, we sort to
	// guarantee best possible match between nodes.
	sort.Strings(servers)
	return c.serverList.SetServers(servers...)
}

func memcacheStatusCode(err error) string {
	// See https://godoc.org/github.com/bradfitz/gomemcache/memcache#pkg-variables
	switch err {
	case nil:
		return "200"
	case memcache.ErrCacheMiss:
		return "404"
	case memcache.ErrMalformedKey:
		return "400"
	default:
		return "500"
	}
}

func memcacheKey(userID, chunkID string) string {
	return fmt.Sprintf("%s/%s", userID, chunkID)
}

// FetchChunkData gets chunks from memcache.
func (c *MemcacheClient) FetchChunkData(userID string, chunks []wire.Chunk) (found []wire.Chunk, missing []wire.Chunk, err error) {
	memcacheRequests.Add(float64(len(chunks)))

	keys := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		keys = append(keys, memcacheKey(userID, chunk.ID))
	}

	var items map[string]*memcache.Item
	err = timeRequestMethodStatus("Get", memcacheRequestDuration, memcacheStatusCode, func() error {
		var err error
		items, err = c.client.GetMulti(keys)
		return err
	})
	if err != nil {
		return nil, chunks, err
	}

	for _, chunk := range chunks {
		item, ok := items[memcacheKey(userID, chunk.ID)]
		if !ok {
			missing = append(missing, chunk)
		} else {
			chunk.Data = item.Value
			found = append(found, chunk)
		}
	}

	memcacheHits.Add(float64(len(found)))
	return found, missing, nil
}

// StoreChunkData serializes and stores a chunk in memcache.
func (c *MemcacheClient) StoreChunkData(userID string, chunk *wire.Chunk) error {
	return timeRequestMethodStatus("Put", memcacheRequestDuration, memcacheStatusCode, func() error {
		// TODO: Add compression - maybe encapsulated in marshaling/unmarshaling
		// methods of wire.Chunk.
		item := memcache.Item{
			Key:        memcacheKey(userID, chunk.ID),
			Value:      chunk.Data,
			Expiration: c.expiration,
		}
		return c.client.Set(&item)
	})
}

// StoreChunks serializes and stores multiple chunks in memcache.
func (c *MemcacheClient) StoreChunks(userID string, chunks []wire.Chunk) error {
	errs := make(chan error)
	for _, chunk := range chunks {
		go func(chunk *wire.Chunk) {
			errs <- c.StoreChunkData(userID, chunk)
		}(&chunk)
	}
	var errOut error
	for i := 0; i < len(chunks); i++ {
		if err := <-errs; err != nil {
			errOut = err
		}
	}
	return errOut
}
