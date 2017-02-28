// Copyright 2017 The Prometheus Authors
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

// The main package for the Prometheus server executable.
package main

import (
	"flag"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	influx "github.com/influxdb/influxdb/client"

	"github.com/prometheus/prometheus/documentation/examples/remote_storage/remote_storage_bridge/graphite"
	"github.com/prometheus/prometheus/documentation/examples/remote_storage/remote_storage_bridge/influxdb"
	"github.com/prometheus/prometheus/documentation/examples/remote_storage/remote_storage_bridge/opentsdb"
	"github.com/prometheus/prometheus/storage/remote"
)

type config struct {
	graphiteAddress         string
	graphiteTransport       string
	graphitePrefix          string
	opentsdbURL             string
	influxdbURL             string
	influxdbRetentionPolicy string
	influxdbUsername        string
	influxdbDatabase        string
	influxdbPassword        string
	remoteTimeout           time.Duration
	listenAddr              string
	telemetryPath           string
}

var (
	receivedSamples = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "received_samples_total",
			Help: "Total number of received samples.",
		},
	)
	sentSamples = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sent_samples_total",
			Help: "Total number of processed samples sent to remote storage.",
		},
		[]string{"remote"},
	)
	failedSamples = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "failed_samples_total",
			Help: "Total number of processed samples which failed on send to remote storage.",
		},
		[]string{"remote"},
	)
	sentBatchDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "sent_batch_duration_seconds",
			Help:    "Duration of sample batch send calls to the remote storage.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"remote"},
	)
)

func init() {
	prometheus.MustRegister(receivedSamples)
	prometheus.MustRegister(sentSamples)
	prometheus.MustRegister(failedSamples)
	prometheus.MustRegister(sentBatchDuration)
}

func main() {
	cfg := parseFlags()
	http.Handle(cfg.telemetryPath, prometheus.Handler())

	clients := buildClients(cfg)
	serve(cfg.listenAddr, clients)
}

func parseFlags() *config {
	cfg := &config{
		influxdbPassword: os.Getenv("INFLUXDB_PW"),
	}

	flag.StringVar(&cfg.graphiteAddress, "graphite-address", "",
		"The host:port of the Graphite server to send samples to. None, if empty.",
	)
	flag.StringVar(&cfg.graphiteTransport, "graphite-transport", "tcp",
		"Transport protocol to use to communicate with Graphite. 'tcp', if empty.",
	)
	flag.StringVar(&cfg.graphitePrefix, "graphite-prefix", "",
		"The prefix to prepend to all metrics exported to Graphite. None, if empty.",
	)
	flag.StringVar(&cfg.opentsdbURL, "opentsdb-url", "",
		"The URL of the remote OpenTSDB server to send samples to. None, if empty.",
	)
	flag.StringVar(&cfg.influxdbURL, "influxdb-url", "",
		"The URL of the remote InfluxDB server to send samples to. None, if empty.",
	)
	flag.StringVar(&cfg.influxdbRetentionPolicy, "influxdb.retention-policy", "default",
		"The InfluxDB retention policy to use.",
	)
	flag.StringVar(&cfg.influxdbUsername, "influxdb.username", "",
		"The username to use when sending samples to InfluxDB. The corresponding password must be provided via the INFLUXDB_PW environment variable.",
	)
	flag.StringVar(&cfg.influxdbDatabase, "influxdb.database", "prometheus",
		"The name of the database to use for storing samples in InfluxDB.",
	)
	flag.DurationVar(&cfg.remoteTimeout, "send-timeout", 30*time.Second,
		"The timeout to use when sending samples to the remote storage.",
	)
	flag.StringVar(&cfg.listenAddr, "web.listen-address", ":9201", "Address to listen on for web endpoints.")
	flag.StringVar(&cfg.telemetryPath, "web.telemetry-path", "/metrics", "Address to listen on for web endpoints.")

	flag.Parse()

	return cfg
}

func buildClients(cfg *config) []remote.StorageClient {
	var clients []remote.StorageClient
	if cfg.graphiteAddress != "" {
		c := graphite.NewClient(
			cfg.graphiteAddress, cfg.graphiteTransport,
			cfg.remoteTimeout, cfg.graphitePrefix)
		clients = append(clients, c)
	}
	if cfg.opentsdbURL != "" {
		c := opentsdb.NewClient(cfg.opentsdbURL, cfg.remoteTimeout)
		clients = append(clients, c)
	}
	if cfg.influxdbURL != "" {
		url, err := url.Parse(cfg.influxdbURL)
		if err != nil {
			log.Fatalf("Failed to parse InfluxDB URL %q: %v", cfg.influxdbURL, err)
		}
		conf := influx.Config{
			URL:      *url,
			Username: cfg.influxdbUsername,
			Password: cfg.influxdbPassword,
			Timeout:  cfg.remoteTimeout,
		}
		c := influxdb.NewClient(conf, cfg.influxdbDatabase, cfg.influxdbRetentionPolicy)
		prometheus.MustRegister(c)
		clients = append(clients, c)
	}
	return clients
}

func serve(addr string, clients []remote.StorageClient) error {
	http.HandleFunc("/receive", func(w http.ResponseWriter, r *http.Request) {
		reqBuf, err := ioutil.ReadAll(snappy.NewReader(r.Body))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var req remote.WriteRequest
		if err := proto.Unmarshal(reqBuf, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		samples := protoToSamples(&req)
		receivedSamples.Add(float64(len(samples)))

		var wg sync.WaitGroup
		for _, c := range clients {
			wg.Add(1)
			go func(rc remote.StorageClient) {
				sendSamples(rc, samples)
				wg.Done()
			}(c)
		}
		wg.Wait()
	})

	return http.ListenAndServe(addr, nil)
}

func protoToSamples(req *remote.WriteRequest) model.Samples {
	var samples model.Samples
	for _, ts := range req.Timeseries {
		metric := make(model.Metric, len(ts.Labels))
		for _, l := range ts.Labels {
			metric[model.LabelName(l.Name)] = model.LabelValue(l.Value)
		}

		for _, s := range ts.Samples {
			samples = append(samples, &model.Sample{
				Metric:    metric,
				Value:     model.SampleValue(s.Value),
				Timestamp: model.Time(s.TimestampMs),
			})
		}
	}
	return samples
}

func sendSamples(c remote.StorageClient, samples model.Samples) {
	begin := time.Now()
	err := c.Store(samples)
	duration := time.Since(begin).Seconds()
	if err != nil {
		log.Warnf("Error sending %d samples to remote storage %q: %v", len(samples), c.Name(), err)
		failedSamples.WithLabelValues(c.Name()).Add(float64(len(samples)))
	}
	sentSamples.WithLabelValues(c.Name()).Add(float64(len(samples)))
	sentBatchDuration.WithLabelValues(c.Name()).Observe(duration)
}
