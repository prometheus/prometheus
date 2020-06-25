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

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunks"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	if err := execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func execute() (err error) {
	var (
		defaultDBPath = filepath.Join("benchout", "storage")

		cli                  = kingpin.New(filepath.Base(os.Args[0]), "CLI tool for tsdb")
		benchCmd             = cli.Command("bench", "run benchmarks")
		benchWriteCmd        = benchCmd.Command("write", "run a write performance benchmark")
		benchWriteOutPath    = benchWriteCmd.Flag("out", "set the output path").Default("benchout").String()
		benchWriteNumMetrics = benchWriteCmd.Flag("metrics", "number of metrics to read").Default("10000").Int()
		benchSamplesFile     = benchWriteCmd.Arg("file", "input file with samples data, default is ("+filepath.Join("..", "..", "testdata", "20kseries.json")+")").Default(filepath.Join("..", "..", "testdata", "20kseries.json")).String()
		listCmd              = cli.Command("ls", "list db blocks")
		listCmdHumanReadable = listCmd.Flag("human-readable", "print human readable values").Short('h').Bool()
		listPath             = listCmd.Arg("db path", "database path (default is "+defaultDBPath+")").Default(defaultDBPath).String()
		analyzeCmd           = cli.Command("analyze", "analyze churn, label pair cardinality.")
		analyzePath          = analyzeCmd.Arg("db path", "database path (default is "+defaultDBPath+")").Default(defaultDBPath).String()
		analyzeBlockID       = analyzeCmd.Arg("block id", "block to analyze (default is the last block)").String()
		analyzeLimit         = analyzeCmd.Flag("limit", "how many items to show in each list").Default("20").Int()
		dumpCmd              = cli.Command("dump", "dump samples from a TSDB")
		dumpPath             = dumpCmd.Arg("db path", "database path (default is "+defaultDBPath+")").Default(defaultDBPath).String()
		dumpMinTime          = dumpCmd.Flag("min-time", "minimum timestamp to dump").Default(strconv.FormatInt(math.MinInt64, 10)).Int64()
		dumpMaxTime          = dumpCmd.Flag("max-time", "maximum timestamp to dump").Default(strconv.FormatInt(math.MaxInt64, 10)).Int64()
	)

	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	var merr tsdb_errors.MultiError

	switch kingpin.MustParse(cli.Parse(os.Args[1:])) {
	case benchWriteCmd.FullCommand():
		wb := &writeBenchmark{
			outPath:     *benchWriteOutPath,
			numMetrics:  *benchWriteNumMetrics,
			samplesFile: *benchSamplesFile,
			logger:      logger,
		}
		return wb.run()
	case listCmd.FullCommand():
		db, err := tsdb.OpenDBReadOnly(*listPath, nil)
		if err != nil {
			return err
		}
		defer func() {
			merr.Add(err)
			merr.Add(db.Close())
			err = merr.Err()
		}()
		blocks, err := db.Blocks()
		if err != nil {
			return err
		}
		printBlocks(blocks, listCmdHumanReadable)
	case analyzeCmd.FullCommand():
		db, err := tsdb.OpenDBReadOnly(*analyzePath, nil)
		if err != nil {
			return err
		}
		defer func() {
			merr.Add(err)
			merr.Add(db.Close())
			err = merr.Err()
		}()
		blocks, err := db.Blocks()
		if err != nil {
			return err
		}
		var block tsdb.BlockReader
		if *analyzeBlockID != "" {
			for _, b := range blocks {
				if b.Meta().ULID.String() == *analyzeBlockID {
					block = b
					break
				}
			}
		} else if len(blocks) > 0 {
			block = blocks[len(blocks)-1]
		}
		if block == nil {
			return fmt.Errorf("block not found")
		}
		return analyzeBlock(block, *analyzeLimit)
	case dumpCmd.FullCommand():
		db, err := tsdb.OpenDBReadOnly(*dumpPath, nil)
		if err != nil {
			return err
		}
		defer func() {
			merr.Add(err)
			merr.Add(db.Close())
			err = merr.Err()
		}()
		return dumpSamples(db, *dumpMinTime, *dumpMaxTime)
	}
	return nil
}

type writeBenchmark struct {
	outPath     string
	samplesFile string
	cleanup     bool
	numMetrics  int

	storage *tsdb.DB

	cpuprof   *os.File
	memprof   *os.File
	blockprof *os.File
	mtxprof   *os.File
	logger    log.Logger
}

func (b *writeBenchmark) run() error {
	if b.outPath == "" {
		dir, err := ioutil.TempDir("", "tsdb_bench")
		if err != nil {
			return err
		}
		b.outPath = dir
		b.cleanup = true
	}
	if err := os.RemoveAll(b.outPath); err != nil {
		return err
	}
	if err := os.MkdirAll(b.outPath, 0777); err != nil {
		return err
	}

	dir := filepath.Join(b.outPath, "storage")

	l := log.With(b.logger, "ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)

	st, err := tsdb.Open(dir, l, nil, &tsdb.Options{
		RetentionDuration: int64(15 * 24 * time.Hour / time.Millisecond),
		MinBlockDuration:  int64(2 * time.Hour / time.Millisecond),
	})
	if err != nil {
		return err
	}
	b.storage = st

	var labels []labels.Labels

	_, err = measureTime("readData", func() error {
		f, err := os.Open(b.samplesFile)
		if err != nil {
			return err
		}
		defer f.Close()

		labels, err = readPrometheusLabels(f, b.numMetrics)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	var total uint64

	dur, err := measureTime("ingestScrapes", func() error {
		if err := b.startProfiling(); err != nil {
			return err
		}
		total, err = b.ingestScrapes(labels, 3000)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Println(" > total samples:", total)
	fmt.Println(" > samples/sec:", float64(total)/dur.Seconds())

	_, err = measureTime("stopStorage", func() error {
		if err := b.storage.Close(); err != nil {
			return err
		}
		if err := b.stopProfiling(); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

const timeDelta = 30000

func (b *writeBenchmark) ingestScrapes(lbls []labels.Labels, scrapeCount int) (uint64, error) {
	var mu sync.Mutex
	var total uint64

	for i := 0; i < scrapeCount; i += 100 {
		var wg sync.WaitGroup
		lbls := lbls
		for len(lbls) > 0 {
			l := 1000
			if len(lbls) < 1000 {
				l = len(lbls)
			}
			batch := lbls[:l]
			lbls = lbls[l:]

			wg.Add(1)
			go func() {
				n, err := b.ingestScrapesShard(batch, 100, int64(timeDelta*i))
				if err != nil {
					// exitWithError(err)
					fmt.Println(" err", err)
				}
				mu.Lock()
				total += n
				mu.Unlock()
				wg.Done()
			}()
		}
		wg.Wait()
	}
	fmt.Println("ingestion completed")

	return total, nil
}

func (b *writeBenchmark) ingestScrapesShard(lbls []labels.Labels, scrapeCount int, baset int64) (uint64, error) {
	ts := baset

	type sample struct {
		labels labels.Labels
		value  int64
		ref    *uint64
	}

	scrape := make([]*sample, 0, len(lbls))

	for _, m := range lbls {
		scrape = append(scrape, &sample{
			labels: m,
			value:  123456789,
		})
	}
	total := uint64(0)

	for i := 0; i < scrapeCount; i++ {
		app := b.storage.Appender()
		ts += timeDelta

		for _, s := range scrape {
			s.value += 1000

			if s.ref == nil {
				ref, err := app.Add(s.labels, ts, float64(s.value))
				if err != nil {
					panic(err)
				}
				s.ref = &ref
			} else if err := app.AddFast(*s.ref, ts, float64(s.value)); err != nil {

				if errors.Cause(err) != storage.ErrNotFound {
					panic(err)
				}

				ref, err := app.Add(s.labels, ts, float64(s.value))
				if err != nil {
					panic(err)
				}
				s.ref = &ref
			}

			total++
		}
		if err := app.Commit(); err != nil {
			return total, err
		}
	}
	return total, nil
}

func (b *writeBenchmark) startProfiling() error {
	var err error

	// Start CPU profiling.
	b.cpuprof, err = os.Create(filepath.Join(b.outPath, "cpu.prof"))
	if err != nil {
		return fmt.Errorf("bench: could not create cpu profile: %v", err)
	}
	if err := pprof.StartCPUProfile(b.cpuprof); err != nil {
		return fmt.Errorf("bench: could not start CPU profile: %v", err)
	}

	// Start memory profiling.
	b.memprof, err = os.Create(filepath.Join(b.outPath, "mem.prof"))
	if err != nil {
		return fmt.Errorf("bench: could not create memory profile: %v", err)
	}
	runtime.MemProfileRate = 64 * 1024

	// Start fatal profiling.
	b.blockprof, err = os.Create(filepath.Join(b.outPath, "block.prof"))
	if err != nil {
		return fmt.Errorf("bench: could not create block profile: %v", err)
	}
	runtime.SetBlockProfileRate(20)

	b.mtxprof, err = os.Create(filepath.Join(b.outPath, "mutex.prof"))
	if err != nil {
		return fmt.Errorf("bench: could not create mutex profile: %v", err)
	}
	runtime.SetMutexProfileFraction(20)
	return nil
}

func (b *writeBenchmark) stopProfiling() error {
	if b.cpuprof != nil {
		pprof.StopCPUProfile()
		b.cpuprof.Close()
		b.cpuprof = nil
	}
	if b.memprof != nil {
		if err := pprof.Lookup("heap").WriteTo(b.memprof, 0); err != nil {
			return fmt.Errorf("error writing mem profile: %v", err)
		}
		b.memprof.Close()
		b.memprof = nil
	}
	if b.blockprof != nil {
		if err := pprof.Lookup("block").WriteTo(b.blockprof, 0); err != nil {
			return fmt.Errorf("error writing block profile: %v", err)
		}
		b.blockprof.Close()
		b.blockprof = nil
		runtime.SetBlockProfileRate(0)
	}
	if b.mtxprof != nil {
		if err := pprof.Lookup("mutex").WriteTo(b.mtxprof, 0); err != nil {
			return fmt.Errorf("error writing mutex profile: %v", err)
		}
		b.mtxprof.Close()
		b.mtxprof = nil
		runtime.SetMutexProfileFraction(0)
	}
	return nil
}

func measureTime(stage string, f func() error) (time.Duration, error) {
	fmt.Printf(">> start stage=%s\n", stage)
	start := time.Now()
	err := f()
	if err != nil {
		return 0, err
	}
	fmt.Printf(">> completed stage=%s duration=%s\n", stage, time.Since(start))
	return time.Since(start), nil
}

func readPrometheusLabels(r io.Reader, n int) ([]labels.Labels, error) {
	scanner := bufio.NewScanner(r)

	var mets []labels.Labels
	hashes := map[uint64]struct{}{}
	i := 0

	for scanner.Scan() && i < n {
		m := make(labels.Labels, 0, 10)

		r := strings.NewReplacer("\"", "", "{", "", "}", "")
		s := r.Replace(scanner.Text())

		labelChunks := strings.Split(s, ",")
		for _, labelChunk := range labelChunks {
			split := strings.Split(labelChunk, ":")
			m = append(m, labels.Label{Name: split[0], Value: split[1]})
		}
		// Order of the k/v labels matters, don't assume we'll always receive them already sorted.
		sort.Sort(m)
		h := m.Hash()
		if _, ok := hashes[h]; ok {
			continue
		}
		mets = append(mets, m)
		hashes[h] = struct{}{}
		i++
	}
	return mets, nil
}

func printBlocks(blocks []tsdb.BlockReader, humanReadable *bool) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer tw.Flush()

	fmt.Fprintln(tw, "BLOCK ULID\tMIN TIME\tMAX TIME\tNUM SAMPLES\tNUM CHUNKS\tNUM SERIES")
	for _, b := range blocks {
		meta := b.Meta()

		fmt.Fprintf(tw,
			"%v\t%v\t%v\t%v\t%v\t%v\n",
			meta.ULID,
			getFormatedTime(meta.MinTime, humanReadable),
			getFormatedTime(meta.MaxTime, humanReadable),
			meta.Stats.NumSamples,
			meta.Stats.NumChunks,
			meta.Stats.NumSeries,
		)
	}
}

func getFormatedTime(timestamp int64, humanReadable *bool) string {
	if *humanReadable {
		return time.Unix(timestamp/1000, 0).UTC().String()
	}
	return strconv.FormatInt(timestamp, 10)
}

func analyzeBlock(b tsdb.BlockReader, limit int) error {
	meta := b.Meta()
	fmt.Printf("Block ID: %s\n", meta.ULID)
	// Presume 1ms resolution that Prometheus uses.
	fmt.Printf("Duration: %s\n", (time.Duration(meta.MaxTime-meta.MinTime) * 1e6).String())
	fmt.Printf("Series: %d\n", meta.Stats.NumSeries)
	ir, err := b.Index()
	if err != nil {
		return err
	}
	defer ir.Close()

	allLabelNames, err := ir.LabelNames()
	if err != nil {
		return err
	}
	fmt.Printf("Label names: %d\n", len(allLabelNames))

	type postingInfo struct {
		key    string
		metric uint64
	}
	postingInfos := []postingInfo{}

	printInfo := func(postingInfos []postingInfo) {
		sort.Slice(postingInfos, func(i, j int) bool { return postingInfos[i].metric > postingInfos[j].metric })

		for i, pc := range postingInfos {
			if i >= limit {
				break
			}
			fmt.Printf("%d %s\n", pc.metric, pc.key)
		}
	}

	labelsUncovered := map[string]uint64{}
	labelpairsUncovered := map[string]uint64{}
	labelpairsCount := map[string]uint64{}
	entries := 0
	p, err := ir.Postings("", "") // The special all key.
	if err != nil {
		return err
	}
	lbls := labels.Labels{}
	chks := []chunks.Meta{}
	for p.Next() {
		if err = ir.Series(p.At(), &lbls, &chks); err != nil {
			return err
		}
		// Amount of the block time range not covered by this series.
		uncovered := uint64(meta.MaxTime-meta.MinTime) - uint64(chks[len(chks)-1].MaxTime-chks[0].MinTime)
		for _, lbl := range lbls {
			key := lbl.Name + "=" + lbl.Value
			labelsUncovered[lbl.Name] += uncovered
			labelpairsUncovered[key] += uncovered
			labelpairsCount[key]++
			entries++
		}
	}
	if p.Err() != nil {
		return p.Err()
	}
	fmt.Printf("Postings (unique label pairs): %d\n", len(labelpairsUncovered))
	fmt.Printf("Postings entries (total label pairs): %d\n", entries)

	postingInfos = postingInfos[:0]
	for k, m := range labelpairsUncovered {
		postingInfos = append(postingInfos, postingInfo{k, uint64(float64(m) / float64(meta.MaxTime-meta.MinTime))})
	}

	fmt.Printf("\nLabel pairs most involved in churning:\n")
	printInfo(postingInfos)

	postingInfos = postingInfos[:0]
	for k, m := range labelsUncovered {
		postingInfos = append(postingInfos, postingInfo{k, uint64(float64(m) / float64(meta.MaxTime-meta.MinTime))})
	}

	fmt.Printf("\nLabel names most involved in churning:\n")
	printInfo(postingInfos)

	postingInfos = postingInfos[:0]
	for k, m := range labelpairsCount {
		postingInfos = append(postingInfos, postingInfo{k, m})
	}

	fmt.Printf("\nMost common label pairs:\n")
	printInfo(postingInfos)

	postingInfos = postingInfos[:0]
	for _, n := range allLabelNames {
		values, err := ir.SortedLabelValues(n)
		if err != nil {
			return err
		}
		var cumulativeLength uint64
		for _, str := range values {
			cumulativeLength += uint64(len(str))
		}
		postingInfos = append(postingInfos, postingInfo{n, cumulativeLength})
	}

	fmt.Printf("\nLabel names with highest cumulative label value length:\n")
	printInfo(postingInfos)

	postingInfos = postingInfos[:0]
	for _, n := range allLabelNames {
		lv, err := ir.SortedLabelValues(n)
		if err != nil {
			return err
		}
		postingInfos = append(postingInfos, postingInfo{n, uint64(len(lv))})
	}
	fmt.Printf("\nHighest cardinality labels:\n")
	printInfo(postingInfos)

	postingInfos = postingInfos[:0]
	lv, err := ir.SortedLabelValues("__name__")
	if err != nil {
		return err
	}
	for _, n := range lv {
		postings, err := ir.Postings("__name__", n)
		if err != nil {
			return err
		}
		count := 0
		for postings.Next() {
			count++
		}
		if postings.Err() != nil {
			return postings.Err()
		}
		postingInfos = append(postingInfos, postingInfo{n, uint64(count)})
	}
	fmt.Printf("\nHighest cardinality metric names:\n")
	printInfo(postingInfos)
	return nil
}

func dumpSamples(db *tsdb.DBReadOnly, mint, maxt int64) (err error) {
	q, err := db.Querier(context.TODO(), mint, maxt)
	if err != nil {
		return err
	}
	defer func() {
		var merr tsdb_errors.MultiError
		merr.Add(err)
		merr.Add(q.Close())
		err = merr.Err()
	}()

	ss := q.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "", ".*"))

	for ss.Next() {
		series := ss.At()
		labels := series.Labels()
		it := series.Iterator()
		for it.Next() {
			ts, val := it.At()
			fmt.Printf("%s %g %d\n", labels, val, ts)
		}
		if it.Err() != nil {
			return ss.Err()
		}
	}

	if ws := ss.Warnings(); len(ws) > 0 {
		var merr tsdb_errors.MultiError
		for _, w := range ws {
			merr.Add(w)
		}
		return merr.Err()
	}

	if ss.Err() != nil {
		return ss.Err()
	}
	return nil
}
