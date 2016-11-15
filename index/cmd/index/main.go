package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/fabxc/tsdb/index"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "index",
		Short: "CLI tool for index",
	}

	root.AddCommand(
		NewBenchCommand(),
	)

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
	outPath string
	cleanup bool

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

	return c
}

func (b *writeBenchmark) run(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		exitWithError(fmt.Errorf("missing file argument"))
	}
	if b.outPath == "" {
		dir, err := ioutil.TempDir("", "index_bench")
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

	var docs []*InsertDoc

	measureTime("readData", func() {
		f, err := os.Open(args[0])
		if err != nil {
			exitWithError(err)
		}
		defer f.Close()

		docs, err = readPrometheusLabels(f)
		if err != nil {
			exitWithError(err)
		}
	})

	dir := filepath.Join(b.outPath, "ix")

	ix, err := index.Open(dir, nil)
	if err != nil {
		exitWithError(err)
	}
	defer func() {
		ix.Close()
		reportSize(dir)
		if b.cleanup {
			os.RemoveAll(b.outPath)
		}
	}()

	measureTime("indexData", func() {
		b.startProfiling()
		indexDocs(ix, docs, 100000)
		indexDocs(ix, docs, 100000)
		indexDocs(ix, docs, 100000)
		indexDocs(ix, docs, 100000)
		b.stopProfiling()
	})
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

func indexDocs(ix *index.Index, docs []*InsertDoc, batchSize int) {
	remDocs := docs[:]
	var ids []index.DocID

	for len(remDocs) > 0 {
		n := batchSize
		if n > len(remDocs) {
			n = len(remDocs)
		}

		b, err := ix.Batch()
		if err != nil {
			exitWithError(err)
		}
		for _, d := range remDocs[:n] {
			id := b.Add(d.Terms)
			ids = append(ids, id)
		}
		if err := b.Commit(); err != nil {
			exitWithError(err)
		}

		remDocs = remDocs[n:]
	}
}

func reportSize(dir string) {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || path == dir {
			return err
		}
		fmt.Printf(" > file=%s size=%.03fGiB\n", path[len(dir):], float64(info.Size())/1024/1024/1024)
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

type InsertDoc struct {
	Terms index.Terms
}

func readPrometheusLabels(r io.Reader) ([]*InsertDoc, error) {
	dec := expfmt.NewDecoder(r, expfmt.FmtProtoText)

	var docs []*InsertDoc
	var mf dto.MetricFamily

	for {
		if err := dec.Decode(&mf); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		for _, m := range mf.GetMetric() {
			d := &InsertDoc{
				Terms: make(index.Terms, len(m.GetLabel())+1),
			}
			d.Terms[0] = index.Term{
				Field: "__name__",
				Val:   mf.GetName(),
			}
			for i, l := range m.GetLabel() {
				d.Terms[i+1] = index.Term{
					Field: l.GetName(),
					Val:   l.GetValue(),
				}
			}
			docs = append(docs, d)
		}
	}

	return docs, nil
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
