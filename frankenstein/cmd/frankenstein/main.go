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

package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/route"
	"github.com/weaveworks/scope/common/middleware"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/frankenstein"
	"github.com/prometheus/prometheus/frankenstein/api"
	"github.com/prometheus/prometheus/storage/local"
)

const (
	distributor = "distributor"
	ingester    = "ingester"
	infName     = "eth0"
)

var (
	requestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "frankenstein",
		Name:      "request_duration_seconds",
		Help:      "Time (in seconds) spent serving HTTP requests.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route", "status_code", "ws"})
)

func init() {
	prometheus.MustRegister(requestDuration)
}

func main() {
	var (
		mode                 string
		listenPort           int
		consulHost           string
		consulPrefix         string
		s3URL                string
		dynamodbURL          string
		dynamodbCreateTables bool
		memcachedHostname    string
		memcachedTimeout     time.Duration
		memcachedExpiration  time.Duration
		memcachedService     string
		remoteTimeout        time.Duration
		flushPeriod          time.Duration
		maxChunkAge          time.Duration
		numTokens            int
	)

	flag.StringVar(&mode, "mode", distributor, "Mode (distributor, ingester).")
	flag.IntVar(&listenPort, "web.listen-port", 9094, "HTTP server listen port.")
	flag.StringVar(&consulHost, "consul.hostname", "localhost:8500", "Hostname and port of Consul.")
	flag.StringVar(&consulPrefix, "consul.prefix", "collectors/", "Prefix for keys in Consul.")
	flag.StringVar(&s3URL, "s3.url", "localhost:4569", "S3 endpoint URL.")
	flag.StringVar(&dynamodbURL, "dynamodb.url", "localhost:8000", "DynamoDB endpoint URL.")
	flag.BoolVar(&dynamodbCreateTables, "dynamodb.create-tables", false, "Create required DynamoDB tables on startup.")
	flag.StringVar(&memcachedHostname, "memcached.hostname", "", "Hostname for memcached service to use when caching chunks. If empty, no memcached will be used.")
	flag.DurationVar(&memcachedTimeout, "memcached.timeout", 100*time.Millisecond, "Maximum time to wait before giving up on memcached requests.")
	flag.DurationVar(&memcachedExpiration, "memcached.expiration", 0, "How long chunks stay in the memcache.")
	flag.StringVar(&memcachedService, "memcached.service", "memcached", "SRV service used to discover memcache servers.")
	flag.DurationVar(&remoteTimeout, "remote.timeout", 5*time.Second, "Timeout for downstream ingesters.")
	flag.DurationVar(&flushPeriod, "ingester.flush-period", 1*time.Minute, "Period with which to attempt to flush chunks.")
	flag.DurationVar(&maxChunkAge, "ingester.max-chunk-age", 10*time.Minute, "Maximum chunk age before flushing.")
	flag.IntVar(&numTokens, "ingester.num-tokens", 128, "Number of tokens for each ingester.")
	flag.Parse()

	consul, err := frankenstein.NewConsulClient(consulHost)
	if err != nil {
		log.Fatalf("Error initializing Consul client: %v", err)
	}

	var memcache *frankenstein.MemcacheClient
	if memcachedHostname != "" {
		memcache = frankenstein.NewMemcacheClient(frankenstein.MemcacheConfig{
			Host:           memcachedHostname,
			Service:        memcachedService,
			Timeout:        memcachedTimeout,
			UpdateInterval: 1 * time.Minute,
			Expiration:     memcachedExpiration,
		})
	}
	chunkStore, err := frankenstein.NewAWSChunkStore(frankenstein.ChunkStoreConfig{
		S3URL:          s3URL,
		DynamoDBURL:    dynamodbURL,
		MemcacheClient: memcache,
	})
	if err != nil {
		log.Fatal(err)
	}
	if dynamodbCreateTables {
		if err = chunkStore.CreateTables(); err != nil {
			log.Fatal(err)
		}
	}

	switch mode {
	case distributor:
		setupDistributor(consul, consulPrefix, chunkStore, remoteTimeout)
	case ingester:
		cfg := local.IngesterConfig{
			FlushCheckPeriod: flushPeriod,
			MaxChunkAge:      maxChunkAge,
		}
		ingester := setupIngester(consul, consulPrefix, chunkStore, listenPort, cfg, numTokens)
		defer ingester.Stop()
		defer frankenstein.DeleteIngesterConfigFromConsul(consul, consulPrefix)
	default:
		log.Fatalf("Mode %s not supported!", mode)
	}

	http.Handle("/metrics", prometheus.Handler())
	go http.ListenAndServe(fmt.Sprintf(":%d", listenPort), nil)

	term := make(chan os.Signal)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	<-term
	log.Warn("Received SIGTERM, exiting gracefully...")
}

func setupDistributor(
	consulClient frankenstein.ConsulClient,
	consulPrefix string,
	chunkStore frankenstein.ChunkStore,
	remoteTimeout time.Duration,
) {
	clientFactory := func(hostname string) (*frankenstein.IngesterClient, error) {
		return frankenstein.NewIngesterClient(hostname, remoteTimeout), nil
	}
	distributor, err := frankenstein.NewDistributor(frankenstein.DistributorConfig{
		ConsulClient:  consulClient,
		ConsulPrefix:  consulPrefix,
		ClientFactory: clientFactory,
	})
	if err != nil {
		log.Fatal(err)
	}
	prometheus.MustRegister(distributor)

	prefix := "/api/prom"
	http.Handle(prefix+"/push", frankenstein.AppenderHandler(distributor))

	// TODO: Move querier to separate binary.
	setupQuerier(distributor, chunkStore, prefix)
}

// setupQuerier sets up a complete querying pipeline:
//
// PromQL -> MergeQuerier -> Distributor -> IngesterQuerier -> Ingester
//              |
//              +----------> ChunkQuerier -> DynamoDB/S3
func setupQuerier(
	distributor *frankenstein.Distributor,
	chunkStore frankenstein.ChunkStore,
	prefix string,
) {
	newQuerier := func(ctx context.Context) local.Querier {
		return frankenstein.MergeQuerier{
			Queriers: []frankenstein.Querier{
				distributor,
				&frankenstein.ChunkQuerier{
					Store: chunkStore,
				},
			},
			Context: ctx,
		}
	}

	api := api.New(newQuerier)
	router := route.New()
	api.Register(router.WithPrefix(prefix + "/api/v1"))
	http.Handle("/", router)

	http.Handle(prefix+"/graph", frankenstein.GraphHandler())
	http.Handle(prefix+"/static/", frankenstein.StaticAssetsHandler(prefix+"/static/"))
}

func setupIngester(
	consulClient frankenstein.ConsulClient,
	consulPrefix string,
	chunkStore frankenstein.ChunkStore,
	listenPort int,
	cfg local.IngesterConfig,
	numTokens int,
) *local.Ingester {
	for i := 0; i < 10; i++ {
		if err := frankenstein.WriteIngesterConfigToConsul(consulClient, consulPrefix, listenPort, numTokens); err == nil {
			break
		} else {
			log.Errorf("Failed to write to consul, sleeping: %v", err)
			time.Sleep(1 * time.Second)
		}
	}

	ingester, err := local.NewIngester(cfg, chunkStore)
	if err != nil {
		log.Fatal(err)
	}
	prometheus.MustRegister(ingester)

	instr := middleware.Merge(
		middleware.Logging,
		middleware.Instrument{
			Duration: requestDuration,
		},
	).Wrap

	http.Handle("/push", instr(frankenstein.AppenderHandler(ingester)))
	http.Handle("/query", instr(frankenstein.QueryHandler(ingester)))
	http.Handle("/label_values", instr(frankenstein.LabelValuesHandler(ingester)))
	return ingester
}
