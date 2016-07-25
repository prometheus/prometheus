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
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	consul "github.com/hashicorp/consul/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/route"

	"github.com/prometheus/prometheus/frankenstein"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/web/api/v1"
)

const (
	distributor = "distributor"
	ingestor    = "ingestor"
	infName     = "eth0"
)

func main() {
	var (
		mode                 string
		listenPort           int
		consulHost           string
		consulPrefix         string
		s3URL                string
		dynamodbURL          string
		dynamodbCreateTables bool
		remoteTimeout        time.Duration
	)

	flag.StringVar(&mode, "mode", distributor, "Mode (distributor, ingestor).")
	flag.IntVar(&listenPort, "web.listen-port", 9094, "HTTP server listen port.")
	flag.StringVar(&consulHost, "consul.hostname", "localhost:8500", "Hostname and port of Consul.")
	flag.StringVar(&consulPrefix, "consul.prefix", "collectors/", "Prefix for keys in Consul.")
	flag.StringVar(&s3URL, "s3.url", "localhost:4569", "S3 endpoint URL.")
	flag.StringVar(&dynamodbURL, "dynamodb.url", "localhost:8000", "DynamoDB endpoint URL.")
	flag.BoolVar(&dynamodbCreateTables, "dynamodb.create-tables", false, "Create required DynamoDB tables on startup.")
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
		setupIngestor(consul, consulPrefix, chunkStore, listenPort)
	default:
		log.Fatalf("Mode %s not supported!", mode)
	}

	http.Handle("/metrics", prometheus.Handler())
	http.ListenAndServe(fmt.Sprintf(":%d", listenPort), nil)
}

func setupDistributor(
	consulClient frankenstein.ConsulClient,
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

		querier := frankenstein.NewIngesterQuerier(fmt.Sprintf("http://%s/query", hostname), 10*time.Second)

		return &frankenstein.IngesterClient{
			Appender: appender,
			Querier:  querier,
		}, nil
	}

	distributor, err := frankenstein.NewDistributor(frankenstein.DistributorConfig{
		ConsulClient:  consulClient,
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

func setupIngestor(
	consulClient frankenstein.ConsulClient,
	consulPrefix string,
	chunkStore frankenstein.ChunkStore,
	listenPort int,
) {
	for i := 0; i < 10; i++ {
		log.Info("Adding ingestor to consul")
		if err := writeIngestorConfigToConsul(consulClient, consulPrefix, listenPort); err == nil {
			break
		} else {
			log.Errorf("Failed to write to consul, sleeping: %v", err)
			time.Sleep(1 * time.Second)
		}
	}

	ingestor, err := local.NewIngestor(chunkStore)
	if err != nil {
		log.Fatal(err)
	}
	prometheus.MustRegister(ingestor)
	http.Handle("/push", frankenstein.AppenderHandler(ingestor))
	http.Handle("/query", frankenstein.QueryHandler(ingestor))
}

// GetFirstAddressOf returns the first IPv4 address of the supplied interface name.
func GetFirstAddressOf(name string) (string, error) {
	inf, err := net.InterfaceByName(name)
	if err != nil {
		return "", err
	}

	addrs, err := inf.Addrs()
	if err != nil {
		return "", err
	}
	if len(addrs) <= 0 {
		return "", fmt.Errorf("No address found for %s", name)
	}

	for _, addr := range addrs {
		switch v := addr.(type) {
		case *net.IPNet:
			if ip := v.IP.To4(); ip != nil {
				return v.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("No address found for %s", name)
}

func writeIngestorConfigToConsul(consulClient frankenstein.ConsulClient, consulPrefix string, listenPort int) error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	addr, err := GetFirstAddressOf(infName)
	if err != nil {
		return err
	}

	tokenHasher := fnv.New64()
	tokenHasher.Write([]byte(hostname))

	buf, err := json.Marshal(frankenstein.Collector{
		Hostname: fmt.Sprintf("%s:%d", addr, listenPort),
		Tokens:   []uint64{tokenHasher.Sum64()},
	})
	if err != nil {
		return err
	}

	_, err = consulClient.Put(&consul.KVPair{
		Key:   consulPrefix + hostname,
		Value: buf,
	}, &consul.WriteOptions{})
	return err
}
