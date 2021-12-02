package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	golog "github.com/go-kit/log"

	"github.com/prometheus/prometheus/tsdb"
)

func main() {
	var (
		outputDir        string
		shardCount       int
		cpuProf          string
		segmentSizeMB    int64
		maxClosingBlocks int
		symbolFlushers   int
	)

	flag.StringVar(&outputDir, "output-dir", ".", "Output directory for new block(s)")
	flag.StringVar(&cpuProf, "cpuprofile", "", "Where to store CPU profile (it not empty)")
	flag.IntVar(&shardCount, "shard-count", 1, "Number of shards for splitting")
	flag.Int64Var(&segmentSizeMB, "segment-file-size", 512, "Size of segment file")
	flag.IntVar(&maxClosingBlocks, "max-closing-blocks", 2, "Number of blocks that can close at once during split compaction")
	flag.IntVar(&symbolFlushers, "symbol-flushers", 4, "Number of symbol flushers used during split compaction")

	flag.Parse()

	logger := golog.NewLogfmtLogger(os.Stderr)

	var blockDirs []string
	var blocks []*tsdb.Block
	for _, d := range flag.Args() {
		s, err := os.Stat(d)
		if err != nil {
			panic(err)
		}
		if !s.IsDir() {
			log.Fatalln("not a directory: ", d)
		}

		blockDirs = append(blockDirs, d)

		b, err := tsdb.OpenBlock(logger, d, nil)
		if err != nil {
			log.Fatalln("failed to open block:", d, err)
		}

		blocks = append(blocks, b)
		defer b.Close()
	}

	if len(blockDirs) == 0 {
		log.Fatalln("no blocks to compact")
	}

	if cpuProf != "" {
		f, err := os.Create(cpuProf)
		if err != nil {
			log.Fatalln(err)
		}

		log.Println("writing to", cpuProf)
		err = pprof.StartCPUProfile(f)
		if err != nil {
			log.Fatalln(err)
		}

		defer pprof.StopCPUProfile()
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	c, err := tsdb.NewLeveledCompactorWithChunkSize(ctx, nil, logger, []int64{0}, nil, segmentSizeMB*1024*1024, nil)
	if err != nil {
		log.Fatalln("creating compator", err)
	}

	opts := tsdb.DefaultConcurrencyOptions()
	opts.MaxClosingBlocks = maxClosingBlocks
	opts.SymbolsFlushersCount = symbolFlushers
	c.SetConcurrencyOptions(opts)

	_, err = c.CompactWithSplitting(outputDir, blockDirs, blocks, uint64(shardCount))
	if err != nil {
		log.Fatalln("compacting", err)
	}
}
