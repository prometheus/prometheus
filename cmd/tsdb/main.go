package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"sync"
	"time"

	"github.com/fabxc/tsdb"
	"github.com/fabxc/tsdb/labels"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "tsdb",
		Short: "CLI tool for tsdb",
	}

	root.AddCommand(
		NewBenchCommand(),
	)

	flag.CommandLine.Set("log.level", "debug")

	root.Execute()
}

func NewBenchCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "bench",
		Short: "run benchmarks",
	}
	c.AddCommand(NewBenchWriteCommand())

	return c
}

type writeBenchmark struct {
	outPath    string
	cleanup    bool
	numMetrics int

	storage *tsdb.DB

	cpuprof   *os.File
	memprof   *os.File
	blockprof *os.File
}

func NewBenchWriteCommand() *cobra.Command {
	var wb writeBenchmark
	c := &cobra.Command{
		Use:   "write <file>",
		Short: "run a write performance benchmark",
		Run:   wb.run,
	}
	c.PersistentFlags().StringVar(&wb.outPath, "out", "benchout/", "set the output path")
	c.PersistentFlags().IntVar(&wb.numMetrics, "metrics", 10000, "number of metrics to read")
	return c
}

func (b *writeBenchmark) run(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		exitWithError(fmt.Errorf("missing file argument"))
	}
	if b.outPath == "" {
		dir, err := ioutil.TempDir("", "tsdb_bench")
		if err != nil {
			exitWithError(err)
		}
		b.outPath = dir
		b.cleanup = true
	}
	if err := os.RemoveAll(b.outPath); err != nil {
		exitWithError(err)
	}
	if err := os.MkdirAll(b.outPath, 0777); err != nil {
		exitWithError(err)
	}

	dir := filepath.Join(b.outPath, "storage")

	st, err := tsdb.Open(dir, nil, nil)
	if err != nil {
		exitWithError(err)
	}
	b.storage = st

	var metrics []model.Metric

	measureTime("readData", func() {
		f, err := os.Open(args[0])
		if err != nil {
			exitWithError(err)
		}
		defer f.Close()

		metrics, err = readPrometheusLabels(f, b.numMetrics)
		if err != nil {
			exitWithError(err)
		}
	})

	defer func() {
		reportSize(dir)
		if b.cleanup {
			os.RemoveAll(b.outPath)
		}
	}()

	measureTime("ingestScrapes", func() {
		b.startProfiling()
		if err := b.ingestScrapes(metrics, 3000); err != nil {
			exitWithError(err)
		}
	})
	measureTime("stopStorage", func() {
		if err := b.storage.Close(); err != nil {
			exitWithError(err)
		}
		b.stopProfiling()
	})
}

func (b *writeBenchmark) ingestScrapes(metrics []model.Metric, scrapeCount int) error {
	var wg sync.WaitGroup

	for len(metrics) > 0 {
		l := 1000
		if len(metrics) < 1000 {
			l = len(metrics)
		}
		batch := metrics[:l]
		metrics = metrics[l:]

		wg.Add(1)
		go func() {
			if err := b.ingestScrapesShard(batch, scrapeCount); err != nil {
				// exitWithError(err)
				fmt.Println(" err", err)
			}
			wg.Done()
		}()
	}

	wg.Wait()
	return nil
}

func (b *writeBenchmark) ingestScrapesShard(metrics []model.Metric, scrapeCount int) error {
	app := b.storage.Appender()
	ts := int64(model.Now())

	type sample struct {
		labels labels.Labels
		value  int64
	}

	scrape := make(map[uint64]*sample, len(metrics))

	for _, m := range metrics {
		lset := make(labels.Labels, 0, len(m))
		for k, v := range m {
			lset = append(lset, labels.Label{Name: string(k), Value: string(v)})
		}
		sort.Sort(lset)

		scrape[lset.Hash()] = &sample{
			labels: lset,
			value:  123456789,
		}
	}

	for i := 0; i < scrapeCount; i++ {
		ts += int64(30000)

		for _, s := range scrape {
			s.value += 1000
			app.Add(s.labels, ts, float64(s.value))
		}
		if err := app.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (b *writeBenchmark) startProfiling() {
	var err error

	// Start CPU profiling.
	b.cpuprof, err = os.Create(filepath.Join(b.outPath, "cpu.prof"))
	if err != nil {
		exitWithError(fmt.Errorf("bench: could not create cpu profile: %v\n", err))
	}
	pprof.StartCPUProfile(b.cpuprof)

	// Start memory profiling.
	b.memprof, err = os.Create(filepath.Join(b.outPath, "mem.prof"))
	if err != nil {
		exitWithError(fmt.Errorf("bench: could not create memory profile: %v\n", err))
	}
	runtime.MemProfileRate = 4096

	// Start fatal profiling.
	b.blockprof, err = os.Create(filepath.Join(b.outPath, "block.prof"))
	if err != nil {
		exitWithError(fmt.Errorf("bench: could not create block profile: %v\n", err))
	}
	runtime.SetBlockProfileRate(1)
}

func (b *writeBenchmark) stopProfiling() {
	if b.cpuprof != nil {
		pprof.StopCPUProfile()
		b.cpuprof.Close()
		b.cpuprof = nil
	}
	if b.memprof != nil {
		pprof.Lookup("heap").WriteTo(b.memprof, 0)
		b.memprof.Close()
		b.memprof = nil
	}
	if b.blockprof != nil {
		pprof.Lookup("block").WriteTo(b.blockprof, 0)
		b.blockprof.Close()
		b.blockprof = nil
		runtime.SetBlockProfileRate(0)
	}
}

func reportSize(dir string) {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || path == dir {
			return err
		}
		if info.Size() < 10*1024*1024 {
			return nil
		}
		fmt.Printf(" > file=%s size=%.04fGiB\n", path[len(dir):], float64(info.Size())/1024/1024/1024)
		return nil
	})
	if err != nil {
		exitWithError(err)
	}
}

func measureTime(stage string, f func()) {
	fmt.Printf(">> start stage=%s\n", stage)
	start := time.Now()
	f()
	fmt.Printf(">> completed stage=%s duration=%s\n", stage, time.Since(start))
}

func readPrometheusLabels(r io.Reader, n int) ([]model.Metric, error) {
	dec := expfmt.NewDecoder(r, expfmt.FmtProtoText)

	var mets []model.Metric
	fps := map[model.Fingerprint]struct{}{}
	var mf dto.MetricFamily
	var dups int

	for i := 0; i < n; {
		if err := dec.Decode(&mf); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		for _, m := range mf.GetMetric() {
			met := make(model.Metric, len(m.GetLabel())+1)
			met["__name__"] = model.LabelValue(mf.GetName())

			for _, l := range m.GetLabel() {
				met[model.LabelName(l.GetName())] = model.LabelValue(l.GetValue())
			}
			if _, ok := fps[met.Fingerprint()]; ok {
				dups++
			} else {
				mets = append(mets, met)
				fps[met.Fingerprint()] = struct{}{}
			}
			i++
		}
	}
	if dups > 0 {
		fmt.Println("dropped duplicate metrics:", dups)
	}
	fmt.Println("read metrics", len(mets[:n]))

	return mets[:n], nil
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
