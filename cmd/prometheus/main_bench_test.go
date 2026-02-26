package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

const expectedValue = 12345678

type fakeScraper struct {
	triggered atomic.Bool
	metrics   string
}

func (s *fakeScraper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.triggered.CompareAndSwap(true, false) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte(s.metrics))
		return
	}
	// Return empty response on subsequent scrapes
	w.WriteHeader(http.StatusOK)
}

type fakeRW2Receiver struct {
	mu                      sync.Mutex
	targetSamples           int
	receivedExpectedSamples int
	receivedRequests        int
	done                    chan struct{}
}

func newFakeRW2Receiver() *fakeRW2Receiver {
	return &fakeRW2Receiver{
		done: make(chan struct{}),
	}
}

func (r *fakeRW2Receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	decoded, err := snappy.Decode(nil, body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var rwReq writev2.Request
	if err := rwReq.Unmarshal(decoded); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	numSamples := 0
	numExpectedSamples := 0
	for _, ts := range rwReq.Timeseries {
		for _, s := range ts.Samples {
			numSamples++
			// Recognize expected samples by unique value - unlikely to be used in other metrics (up, scrape_..).
			// Easier and cheaper than decoding labels.
			if s.V() == expectedValue {
				numExpectedSamples++
			}
		}
	}

	r.mu.Lock()
	if r.targetSamples > 0 {
		r.receivedRequests++
		r.receivedExpectedSamples += numExpectedSamples
		if r.receivedExpectedSamples >= r.targetSamples {
			r.targetSamples = 0
			close(r.done)
		}
	}
	r.mu.Unlock()

	w.Header().Set("X-Prometheus-Remote-Write-Samples-Written", fmt.Sprintf("%d", numSamples))
	w.Header().Set("X-Prometheus-Remote-Write-Exemplars-Written", "0")
	w.Header().Set("X-Prometheus-Remote-Write-Histograms-Written", "0")
	w.WriteHeader(http.StatusNoContent)
}

func (r *fakeRW2Receiver) reset(target int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.targetSamples = target
	r.receivedRequests = 0
	r.receivedExpectedSamples = 0
	r.done = make(chan struct{})
}

func (r *fakeRW2Receiver) wait(b *testing.B, timeout time.Duration) {
	r.mu.Lock()
	done := r.done
	r.mu.Unlock()

	select {
	case <-done:
		b.ReportMetric(float64(r.receivedRequests), "recv-requests")
		b.ReportMetric(float64(r.receivedExpectedSamples), "recv-samples")
	case <-time.After(timeout):
		r.mu.Lock()
		close(done)
		err := fmt.Errorf("timed out waiting on receiver got %v samples", r.receivedExpectedSamples)
		r.mu.Unlock()
		b.Error(err) // Don't panic: This happens from time to time (main_bench_test.go:126: timed out waiting on receiver got 999000 samples, not sure why).
	}
}

// BenchmarkE2EScrapeAndRemoteWriteNoChurn benchmarks scrape -> WAL -> RW2 send path.
// * Start 1K targets.
// * Starts fake receiver
// * Starts Prometheus (directly invoking main() for accurate CPU/mem statistics and profiles) (HACKY! uses os.Exit etc)
// For each loop this test:
// 1. emits 1K metrics once per each target.
// 2. Expects receiver to consume 1K*1K (so 1M) samples.
//
// Recommended CLI invocation(s):
/*
	export bench=e2erw && go test ./cmd/prometheus/... \
		-run '^$' -bench '^BenchmarkE2EScrapeAndRemoteWriteNoChurn' \
		-benchtime 50x -count 7 -cpu 2 -timeout 999m -benchmem \
		| tee ${bench}.txt
*/
func BenchmarkE2EScrapeAndRemoteWriteNoChurn(b *testing.B) {
	const (
		numTargets       = 1000
		metricsPerTarget = 1000
	)

	rw := newFakeRW2Receiver()
	rwSrv := httptest.NewServer(rw)
	b.Cleanup(rwSrv.Close)

	var scrapers []*fakeScraper
	var targetURLs []string

	for i := 0; i < numTargets; i++ {
		var sb strings.Builder
		for j := 0; j < metricsPerTarget; j++ {
			// e.g. test_metric_0{target="0"} <expectedValue>
			sb.WriteString(fmt.Sprintf("test_metric_%d{target=\"%d\"} %d\n", j, i, expectedValue))
		}
		s := &fakeScraper{metrics: sb.String()}
		scrapers = append(scrapers, s)
		srv := httptest.NewServer(s)
		b.Cleanup(srv.Close)
		targetURLs = append(targetURLs, srv.URL)
	}

	tmpDir := b.TempDir()
	configFile := filepath.Join(tmpDir, "prometheus.yml")

	var config strings.Builder
	config.WriteString(`
global:
  scrape_interval: 1s
remote_write:
  - url: ` + rwSrv.URL + `
    remote_timeout: 30s
    send_exemplars: false
    send_native_histograms: false
    protobuf_message: io.prometheus.write.v2.Request
scrape_configs:
`)

	for i, u := range targetURLs {
		config.WriteString(fmt.Sprintf(`  - job_name: 'job_%d'
    static_configs:
      - targets: ['%s']
`, i, strings.TrimPrefix(u, "http://")))
	}
	require.NoError(b, os.WriteFile(configFile, []byte(config.String()), 0o644))

	promHostPort := "localhost:1234" // TODO: find random open port." + promHostPort,

	// Intercept os.Args and replace with our benchmark args.
	oldArgs := os.Args
	os.Args = []string{
		"prometheus",
		"--config.file=" + configFile,
		"--storage.tsdb.path=" + filepath.Join(tmpDir, "data"),
		"--web.listen-address=" + promHostPort,
		"--log.level=error",
		"--web.enable-lifecycle",
	}

	// Because main() executes global MustRegister calls, we unregister known collectors
	// to prevent panic if this benchmark is executed multiple times (e.g., manually via test runner repeatedly).
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
	// prometheus.Unregister(collectors.NewGoCollector())
	// prometheus.Unregister(configSuccess)
	// prometheus.Unregister(configSuccessTime)

	var wg sync.WaitGroup
	wg.Go(func() {
		// Catch any panic out of main (like duplicate TSDB metric registration)
		// and allow the benchmark to at least report if it happens.
		defer func() {
			if r := recover(); r != nil {
				b.Fatalf("main() panicked: %v\n", r)
			}
		}()
		// Obviously os.Exits will be not handled correctly - they will exit benchmark abruptly.
		main()
	})
	b.Cleanup(func() {
		// Stop prometheus gracefully.
		resp, err := http.Post("http://"+promHostPort+"/-/quit", "text/plain", nil)
		if err != nil {
			b.Log("err when closing:", err)
			return
		}
		_ = resp.Body.Close()
		wg.Wait()
		os.Args = oldArgs
	})

	readyURL := fmt.Sprintf("http://%s/-/ready", promHostPort)
	require.Eventually(b, func() bool {
		resp, err := http.Get(readyURL)
		if err != nil {
			// fmt.Println(">> Waiting for Prometheus to start; readiness", err)
			return false
		}
		// fmt.Println(">> Waiting for Prometheus to start; readiness", resp.StatusCode)
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 10*time.Second, 100*time.Millisecond)

	// Perform a single bench loop before measuring - to warm caches.
	// We expect numTargets * metricsPerTarget metrics to be written (excluding built-in metrics).
	rw.reset(numTargets * metricsPerTarget)
	// Trigger all scrapers.
	for _, s := range scrapers {
		s.triggered.Store(true)
	}
	// Wait until RW2 endpoint receives all metrics.
	rw.wait(b, 2*time.Minute)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		// We expect numTargets * metricsPerTarget metrics to be written.
		rw.reset(numTargets * metricsPerTarget)
		// Trigger all scrapers.
		for _, s := range scrapers {
			s.triggered.Store(true)
		}
		// Wait until RW2 endpoint receives all metrics.
		rw.wait(b, 2*time.Minute)
	}
}
