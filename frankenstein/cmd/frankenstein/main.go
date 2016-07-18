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
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"

	"github.com/prometheus/prometheus/frankenstein"
	"github.com/prometheus/prometheus/frankenstein/wire"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/web/api/v1"
)

const (
	distributor = "distributor"
	ingestor    = "ingestor"
)

func main() {
	var (
		mode                 string
		listen               string
		consulHost           string
		consulPrefix         string
		s3URL                string
		dynamodbURL          string
		dynamodbCreateTables bool
		remoteTimeout        time.Duration
	)

	flag.StringVar(&mode, "mode", distributor, "Mode (distributor, ingestor).")
	flag.StringVar(&listen, "web.listen-address", ":9094", "HTTP server listen address.")
	flag.StringVar(&consulHost, "consul.hostname", "localhost:8500", "Hostname and port of Consul.")
	flag.StringVar(&consulPrefix, "consul.prefix", "collectors/", "Prefix for keys in Consul.")
	flag.StringVar(&s3URL, "s3.url", "localhost:4569", "S3 endpoint URL.")
	flag.StringVar(&dynamodbURL, "dynamodb.url", "localhost:8000", "DynamoDB endpoint URL.")
	flag.BoolVar(&dynamodbCreateTables, "dynamodb.create-tables", true, "Create required DynamoDB tables on startup.")
	flag.DurationVar(&remoteTimeout, "remote.timeout", 100*time.Millisecond, "Timeout for downstream injestors.")
	flag.Parse()

	consul, err := frankenstein.NewConsulClient(consulHost)
	if err != nil {
		log.Fatalf("Error initializing Consul client: %v", err)
	}

	chunkStore, err := frankenstein.NewAWSChunkStore(frankenstein.ChunkStoreConfig{
		S3URL:       s3URL,
		DynamoDBURL: dynamodbURL,
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
	case ingestor:
		setupIngestor(consul, chunkStore)
	default:
		log.Fatalf("Mode %s not supported!", mode)
	}

	http.ListenAndServe(listen, nil)
}

func setupDistributor(
	consul frankenstein.ConsulClient,
	consulPrefix string,
	chunkStore frankenstein.ChunkStore,
	remoteTimeout time.Duration,
) {
	clientFactory := func(hostname string) (*frankenstein.IngesterClient, error) {
		appender := remote.New(&remote.Options{
			GenericURL:     fmt.Sprintf("http://%s/push", hostname),
			StorageTimeout: remoteTimeout,
		})
		appender.Run()

		querier, err := frankenstein.NewIngesterQuerier(fmt.Sprintf("http://%s/", hostname))
		if err != nil {
			return nil, err
		}

		return &frankenstein.IngesterClient{
			Appender: appender,
			Querier:  querier,
		}, nil
	}

	distributor, err := frankenstein.NewDistributor(frankenstein.DistributorConfig{
		Consul:        consul,
		ConsulPrefix:  consulPrefix,
		ClientFactory: clientFactory,
	})
	if err != nil {
		log.Fatal(err)
	}

	// This sets up a complete querying pipeline:
	//
	// PromQL -> MergeQuerier -> Distributor -> IngesterQuerier -> Ingester
	//              |
	//              +----------> ChunkQuerier -> DynamoDB/S3
	//
	// TODO: Move querier to separate binary.

	// Insert some test chunks into DynamoDB/S3.
	writeTestChunks(chunkStore)

	querier := frankenstein.MergeQuerier{
		Queriers: []frankenstein.Querier{
			distributor,
			&frankenstein.ChunkQuerier{
				Store: chunkStore,
			},
		},
	}
	engine := promql.NewEngine(nil, querier, nil)

	api := v1.NewAPI(engine, nil)
	router := route.New()
	api.Register(router.WithPrefix("/api/v1"))
	http.Handle("/", router)

	http.Handle("/push", frankenstein.AppenderHandler(distributor))
}

func setupIngestor(_ frankenstein.ConsulClient, chunkStore frankenstein.ChunkStore) {
	ingestor, err := local.NewIngestor(chunkStore)
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/push", frankenstein.AppenderHandler(ingestor))
}

func writeTestChunks(cs frankenstein.ChunkStore) {
	end := model.Now()
	fooSamples := []model.SamplePair{
		{
			Timestamp: end.Add(-2 * time.Minute),
			Value:     1,
		},
		{
			Timestamp: end.Add(-time.Minute),
			Value:     2,
		},
		{
			Timestamp: end,
			Value:     3,
		},
	}
	barSamples := []model.SamplePair{
		{
			Timestamp: end.Add(-2 * time.Minute),
			Value:     4,
		},
		{
			Timestamp: end.Add(-time.Minute),
			Value:     5,
		},
		{
			Timestamp: end,
			Value:     6,
		},
	}
	err := cs.Put(
		[]wire.Chunk{
			{
				ID:      "0000000000000001",
				From:    fooSamples[0].Timestamp,
				Through: fooSamples[len(fooSamples)-1].Timestamp,
				Metric: model.Metric{
					model.MetricNameLabel: "testmetric",
					"service":             "foo",
				},
				Data: local.EncodeDoubleDeltaChunk(fooSamples),
			},
			{
				ID:      "0000000000000002",
				From:    barSamples[0].Timestamp,
				Through: barSamples[len(barSamples)-1].Timestamp,
				Metric: model.Metric{
					model.MetricNameLabel: "testmetric",
					"service":             "bar",
				},
				Data: local.EncodeDoubleDeltaChunk(barSamples),
			},
		},
	)
	if err != nil {
		log.Fatal(err)
	}
}
