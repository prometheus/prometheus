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
	"fmt"
	"io"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"

	"github.com/prometheus/prometheus/documentation/examples/remote_storage/remote_storage_adapter/graphite"
	"github.com/prometheus/prometheus/documentation/examples/remote_storage/remote_storage_adapter/influxdb"
	"github.com/prometheus/prometheus/documentation/examples/remote_storage/remote_storage_adapter/opentsdb"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
)

type config struct {
	graphiteAddress   string
	graphiteTransport string
	graphitePrefix    string
	opentsdbURL       string
	influxdbURL       string
	bucket            string
	organization      string
	influxdbAuthToken string
	remoteTimeout     time.Duration
	listenAddr        string
	telemetryPath     string
	promslogConfig    promslog.Config
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
			Name:                            "sent_batch_duration_seconds",
			Help:                            "Duration of sample batch send calls to the remote storage.",
			Buckets:                         prometheus.DefBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
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
	http.Handle(cfg.telemetryPath, promhttp.Handler())

	logger := promslog.New(&cfg.promslogConfig)

	writers, readers := buildClients(logger, cfg)
	if err := serve(logger, cfg.listenAddr, writers, readers); err != nil {
		logger.Error("Failed to listen", "addr", cfg.listenAddr, "err", err)
		os.Exit(1)
	}
}

func parseFlags() *config {
	a := kingpin.New(filepath.Base(os.Args[0]), "Remote storage adapter")
	a.HelpFlag.Short('h')

	cfg := &config{
		influxdbAuthToken: os.Getenv("INFLUXDB_AUTH_TOKEN"),
		promslogConfig:    promslog.Config{},
	}

	a.Flag("graphite-address", "The host:port of the Graphite server to send samples to. None, if empty.").
		Default("").StringVar(&cfg.graphiteAddress)
	a.Flag("graphite-transport", "Transport protocol to use to communicate with Graphite. 'tcp', if empty.").
		Default("tcp").StringVar(&cfg.graphiteTransport)
	a.Flag("graphite-prefix", "The prefix to prepend to all metrics exported to Graphite. None, if empty.").
		Default("").StringVar(&cfg.graphitePrefix)
	a.Flag("opentsdb-url", "The URL of the remote OpenTSDB server to send samples to. None, if empty.").
		Default("").StringVar(&cfg.opentsdbURL)
	a.Flag("influxdb-url", "The URL of the remote InfluxDB server to send samples to. None, if empty.").
		Default("").StringVar(&cfg.influxdbURL)
	a.Flag("influxdb.bucket", "The InfluxDB bucket to use.").
		Default("").StringVar(&cfg.bucket)
	a.Flag("influxdb.organization", "The name of the organization to use for storing samples in InfluxDB.").
		Default("").StringVar(&cfg.organization)
	a.Flag("send-timeout", "The timeout to use when sending samples to the remote storage.").
		Default("30s").DurationVar(&cfg.remoteTimeout)
	a.Flag("web.listen-address", "Address to listen on for web endpoints.").
		Default(":9201").StringVar(&cfg.listenAddr)
	a.Flag("web.telemetry-path", "Address to listen on for web endpoints.").
		Default("/metrics").StringVar(&cfg.telemetryPath)

	flag.AddFlags(a, &cfg.promslogConfig)

	_, err := a.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing commandline arguments: %s", err)
		a.Usage(os.Args[1:])
		os.Exit(2)
	}

	return cfg
}

type writer interface {
	Write(samples model.Samples) error
	Name() string
}

type reader interface {
	Read(req *prompb.ReadRequest) (*prompb.ReadResponse, error)
	Name() string
}

func buildClients(logger *slog.Logger, cfg *config) ([]writer, []reader) {
	var writers []writer
	var readers []reader
	if cfg.graphiteAddress != "" {
		c := graphite.NewClient(
			logger.With("storage", "Graphite"),
			cfg.graphiteAddress, cfg.graphiteTransport,
			cfg.remoteTimeout, cfg.graphitePrefix)
		writers = append(writers, c)
	}
	if cfg.opentsdbURL != "" {
		c := opentsdb.NewClient(
			logger.With("storage", "OpenTSDB"),
			cfg.opentsdbURL,
			cfg.remoteTimeout,
		)
		writers = append(writers, c)
	}
	if cfg.influxdbURL != "" {
		url, err := url.Parse(cfg.influxdbURL)
		if err != nil {
			logger.Error("Failed to parse InfluxDB URL", "url", cfg.influxdbURL, "err", err)
			os.Exit(1)
		}
		c := influxdb.NewClient(
			logger.With("storage", "InfluxDB"),
			url.String(),
			cfg.influxdbAuthToken,
			cfg.organization,
			cfg.bucket,
		)
		prometheus.MustRegister(c)
		writers = append(writers, c)
		readers = append(readers, c)
	}
	logger.Info("Starting up...")
	return writers, readers
}

func serve(logger *slog.Logger, addr string, writers []writer, readers []reader) error {
	http.HandleFunc("/write", func(w http.ResponseWriter, r *http.Request) {
		req, err := remote.DecodeWriteRequest(r.Body)
		if err != nil {
			logger.Error("Read error", "err", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		samples := protoToSamples(req)
		receivedSamples.Add(float64(len(samples)))

		var wg sync.WaitGroup
		for _, w := range writers {
			wg.Add(1)
			go func(rw writer) {
				sendSamples(logger, rw, samples)
				wg.Done()
			}(w)
		}
		wg.Wait()
	})

	http.HandleFunc("/read", func(w http.ResponseWriter, r *http.Request) {
		compressed, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Error("Read error", "err", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		reqBuf, err := snappy.Decode(nil, compressed)
		if err != nil {
			logger.Error("Decode error", "err", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var req prompb.ReadRequest
		if err := proto.Unmarshal(reqBuf, &req); err != nil {
			logger.Error("Unmarshal error", "err", err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// TODO: Support reading from more than one reader and merging the results.
		if len(readers) != 1 {
			http.Error(w, fmt.Sprintf("expected exactly one reader, found %d readers", len(readers)), http.StatusInternalServerError)
			return
		}
		reader := readers[0]

		var resp *prompb.ReadResponse
		resp, err = reader.Read(&req)
		if err != nil {
			logger.Warn("Error executing query", "query", req, "storage", reader.Name(), "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data, err := proto.Marshal(resp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/x-protobuf")
		w.Header().Set("Content-Encoding", "snappy")

		compressed = snappy.Encode(nil, data)
		if _, err := w.Write(compressed); err != nil {
			logger.Warn("Error writing response", "storage", reader.Name(), "err", err)
		}
	})

	return http.ListenAndServe(addr, nil)
}

func protoToSamples(req *prompb.WriteRequest) model.Samples {
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
				Timestamp: model.Time(s.Timestamp),
			})
		}
	}
	return samples
}

func sendSamples(logger *slog.Logger, w writer, samples model.Samples) {
	begin := time.Now()
	err := w.Write(samples)
	duration := time.Since(begin).Seconds()
	if err != nil {
		logger.Warn("Error sending samples to remote storage", "err", err, "storage", w.Name(), "num_samples", len(samples))
		failedSamples.WithLabelValues(w.Name()).Add(float64(len(samples)))
	}
	sentSamples.WithLabelValues(w.Name()).Add(float64(len(samples)))
	sentBatchDuration.WithLabelValues(w.Name()).Observe(duration)
}
