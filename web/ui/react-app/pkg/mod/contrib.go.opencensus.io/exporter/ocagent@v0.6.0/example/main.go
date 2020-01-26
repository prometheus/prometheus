package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"contrib.go.opencensus.io/exporter/ocagent"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.opencensus.io/trace"
	"go.opencensus.io/zpages"
)

func main() {
	// For the purposes of debugging, we'll add zPages that you can
	// use as a diagnostic to examine if stats and traces are exported
	// out. You can learn about using zPages at https://opencensus.io/zpages/go/
	zPagesMux := http.NewServeMux()
	zpages.Handle(zPagesMux, "/debug")
	go func() {
		if err := http.ListenAndServe(":9999", zPagesMux); err != nil {
			log.Fatalf("Failed to serve zPages")
		}
	}()

	oce, err := ocagent.NewExporter(
		ocagent.WithInsecure(),
		ocagent.WithReconnectionPeriod(5*time.Second),
		ocagent.WithAddress("localhost:55678"), // Only included here for demo purposes.
		ocagent.WithServiceName("ocagent-go-example"))
	if err != nil {
		log.Fatalf("Failed to create ocagent-exporter: %v", err)
	}
	trace.RegisterExporter(oce)
	view.RegisterExporter(oce)

	// Some configurations to get observability signals out.
	view.SetReportingPeriod(7 * time.Second)
	trace.ApplyConfig(trace.Config{
		DefaultSampler: trace.AlwaysSample(),
	})

	// Some stats
	keyClient, _ := tag.NewKey("client")
	keyMethod, _ := tag.NewKey("method")

	mLatencyMs := stats.Float64("latency", "The latency in milliseconds", "ms")
	mLineLengths := stats.Int64("line_lengths", "The length of each line", "By")

	views := []*view.View{
		{
			Name:        "opdemo/latency",
			Description: "The various latencies of the methods",
			Measure:     mLatencyMs,
			Aggregation: view.Distribution(0, 10, 50, 100, 200, 400, 800, 1000, 1400, 2000, 5000, 10000, 15000),
			TagKeys:     []tag.Key{keyClient, keyMethod},
		},
		{
			Name:        "opdemo/process_counts",
			Description: "The various counts",
			Measure:     mLatencyMs,
			Aggregation: view.Count(),
			TagKeys:     []tag.Key{keyClient, keyMethod},
		},
		{
			Name:        "opdemo/line_lengths",
			Description: "The lengths of the various lines in",
			Measure:     mLineLengths,
			Aggregation: view.Distribution(0, 10, 20, 50, 100, 150, 200, 500, 800),
			TagKeys:     []tag.Key{keyClient, keyMethod},
		},
		{
			Name:        "opdemo/line_counts",
			Description: "The counts of the lines in",
			Measure:     mLineLengths,
			Aggregation: view.Count(),
			TagKeys:     []tag.Key{keyClient, keyMethod},
		},
	}

	if err := view.Register(views...); err != nil {
		log.Fatalf("Failed to register views for metrics: %v", err)
	}

	ctx, _ := tag.New(context.Background(), tag.Insert(keyMethod, "repl"), tag.Insert(keyClient, "cli"))
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for {
		startTime := time.Now()
		_, span := trace.StartSpan(context.Background(), "Foo")
		var sleep int64
		switch modulus := time.Now().Unix() % 5; modulus {
		case 0:
			sleep = rng.Int63n(17001)
		case 1:
			sleep = rng.Int63n(8007)
		case 2:
			sleep = rng.Int63n(917)
		case 3:
			sleep = rng.Int63n(87)
		case 4:
			sleep = rng.Int63n(1173)
		}

		time.Sleep(time.Duration(sleep) * time.Millisecond)

		span.End()
		latencyMs := float64(time.Since(startTime)) / 1e6
		nr := int(rng.Int31n(7))
		for i := 0; i < nr; i++ {
			randLineLength := rng.Int63n(999)
			stats.Record(ctx, mLineLengths.M(randLineLength))
			fmt.Printf("#%d: LineLength: %dBy\n", i, randLineLength)
		}
		stats.Record(ctx, mLatencyMs.M(latencyMs))
		fmt.Printf("Latency: %.3fms\n", latencyMs)
	}
}
