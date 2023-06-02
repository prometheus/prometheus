package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"path"
	"sort"
	"time"

	"github.com/alecthomas/kong"
	"github.com/pierrec/lz4"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"go.uber.org/zap"
)

type confS struct {
	pageSize           int
	scaleFactor        int
	blockDir           string
	blockName          string
	outDir             string
	sortOrder          []string
	whiteJobLabelValue map[string]struct{}
}

//nolint:gocyclo // why not?
func main() {
	// init logger
	logger, errL := zap.NewDevelopment()
	if errL != nil {
		log.Fatalf("can't initialize zap logger: %v", errL)
	}

	// init config
	conf := initConfig(logger)

	start := time.Now()
	block, err := tsdb.OpenBlock(nil, path.Join(conf.blockDir, conf.blockName), nil)
	if err != nil {
		logger.Fatal("failed init block", zap.Error(err))
	}

	firstTS := block.Meta().MinTime
	indexReader, err := block.Index()
	if err != nil {
		logger.Fatal("failed to read index", zap.Error(err))
	}

	series, err := getLabelsSets(logger, block, indexReader, conf.whiteJobLabelValue)
	if err != nil {
		logger.Fatal("failed to get label sets", zap.Error(err))
	}
	logger.Info("prepare data", zap.Duration("get labels taken", time.Since(start)))

	var totalTime time.Duration
	totalTime += time.Since(start)

	start = time.Now()
	tsIndex, lsIndex, sByTS, sByLS := getSortedSamples(logger, block, &series, firstTS, conf.scaleFactor)
	logger.Info("prepare data", zap.Duration("get tsDelta and values taken", time.Since(start)))
	totalTime += time.Since(start)

	for _, sortOrder := range conf.sortOrder {
		start = time.Now()
		var outFileName string
		switch sortOrder {
		case "TS_LS": // default sort
			sort.Slice(tsIndex, func(i, j int) bool { return tsIndex[i] < tsIndex[j] })
			for _, ts := range tsIndex {
				sort.Slice(sByTS[ts], func(i, j int) bool { return (series)[sByTS[ts][i].id].s < (series)[sByTS[ts][j].id].s })
			}
			outFileName = "dummy_wal.ts_ls.bin.lz4"
			totalSamplesCount := writeSampleSortedByTs(
				logger,
				conf.pageSize,
				path.Join(conf.outDir, outFileName),
				series,
				tsIndex,
				sByTS,
				firstTS,
			)
			logger.Info("save", zap.Int64("samples total convert", totalSamplesCount))
		case "TS_RLS": // sort by tsDelta and random ls
			sort.Slice(tsIndex, func(i, j int) bool { return tsIndex[i] < tsIndex[j] })
			for _, ts := range tsIndex {
				rand.Shuffle(len(sByTS[ts]), func(i, j int) { sByTS[ts][i], sByTS[ts][j] = sByTS[ts][j], sByTS[ts][i] })
			}
			outFileName = "dummy_wal.ts_Rls.bin.lz4"
			totalSamplesCount := writeSampleSortedByTs(
				logger,
				conf.pageSize,
				path.Join(conf.outDir, outFileName),
				series,
				tsIndex,
				sByTS,
				firstTS,
			)
			logger.Info("save", zap.Int64("samples total convert", totalSamplesCount))
		case "LS_TS":
			tsIndex = nil
			sByTS = nil
			sort.Slice(lsIndex, func(i, j int) bool { return (series)[lsIndex[i]].s < (series)[lsIndex[j]].s })
			for _, id := range lsIndex {
				sort.Slice(sByLS[id], func(i, j int) bool { return sByLS[id][i].tsDelta < sByLS[id][j].tsDelta })
			}
			outFileName = "dummy_wal.ls_ts.bin.lz4"
			totalSamplesCount := writeSampleSortedByLs(
				logger,
				conf.pageSize,
				path.Join(conf.outDir, outFileName),
				series,
				lsIndex,
				sByLS,
				firstTS,
			)
			logger.Info("save", zap.Int64("samples total convert", totalSamplesCount))
		case "LS_RTS":
			tsIndex = nil
			sByTS = nil
			sort.Slice(lsIndex, func(i, j int) bool { return (series)[lsIndex[i]].s < (series)[lsIndex[j]].s })
			for _, id := range lsIndex {
				rand.Shuffle(len(sByLS[id]), func(i, j int) { sByLS[id][i], sByLS[id][j] = sByLS[id][j], sByLS[id][i] })
			}
			outFileName = "dummy_wal.ls_Rts.bin.lz4"
			totalSamplesCount := writeSampleSortedByLs(
				logger,
				conf.pageSize,
				path.Join(conf.outDir, outFileName),
				series,
				lsIndex,
				sByLS,
				firstTS,
			)
			logger.Info("save", zap.Int64("samples total convert", totalSamplesCount))
		case "R":
			tsIndex = nil
			sByTS = nil
			samples := make([]sample, 0)
			for _, id := range lsIndex {
				for i := range sByLS[id] {
					samples = append(samples, sample{id: id, value: sByLS[id][i].value, tsDelta: sByLS[id][i].tsDelta})
				}
			}
			lsIndex = nil
			sByLS = nil
			rand.Shuffle(len(samples), func(i, j int) { samples[i], samples[j] = samples[j], samples[i] })

			outFileName = "dummy_wal.R.bin.lz4"
			totalSamplesCount := writeSamples(
				logger,
				conf.pageSize,
				path.Join(conf.outDir, outFileName),
				series,
				samples,
				firstTS,
			)
			logger.Info("save", zap.Int64("samples total convert", totalSamplesCount))
		}

		logger.Info("save", zap.Duration("save taken", time.Since(start)))
		totalTime += time.Since(start)
	}

	logger.Info("summary", zap.Duration("total time", totalTime))
}

func initConfig(logger *zap.Logger) confS {
	var cmdFlags struct {
		PromBlock struct {
			Dir  string `default:"in" help:"Directory name where stored input Prometheus block."`
			Name string `default:"block" help:"Converted block name."`
		} `embed:"." prefix:"prom-block."`

		Out struct {
			Dir string `default:"out" help:"Directory name where data will be saved."`
		} `embed:"." prefix:"out."`

		Common struct {
			SortOrder          string   `default:"LS_TS" help:"Sort order in result. Available: TS_LS, TS_RLS, RTS_LS, R and ALL."`
			ScaleFactor        int      `default:"1" help:"Scale factor for the count of samples in result." `
			WhiteJobLabelValue []string `default:"machine-controller-manager, image-availability-exporter, extended-monitoring-exporter, kube-dns, deckhouse, node-local-dns, kube-controller-manager, prometheus-operator, kube-apiserver, kube-scheduler, kubelet, kube-etcd3, kube-proxy, cert-manager, nginx-ingress-controller, trickster, prometheus, node-exporter, kube-state-metrics, grafana" help:"Scale factor for the count of samples in result." `
		} `embed:""`
	}

	kong.Parse(&cmdFlags)
	whiteJobLabelValue := make(map[string]struct{})
	for _, v := range cmdFlags.Common.WhiteJobLabelValue {
		whiteJobLabelValue[v] = struct{}{}
	}
	var sortOrder []string
	switch cmdFlags.Common.SortOrder {
	case "TS_LS":
		sortOrder = append(sortOrder, "TS_LS")
	case "TS_RLS":
		sortOrder = append(sortOrder, "TS_RLS")
	case "LS_TS":
		sortOrder = append(sortOrder, "LS_TS")
	case "LS_RTS":
		sortOrder = append(sortOrder, "LS_RTS")
	case "R":
		sortOrder = append(sortOrder, "R")
	case "ALL":
		sortOrder = append(sortOrder, "TS_LS")
		sortOrder = append(sortOrder, "TS_RLS")
		sortOrder = append(sortOrder, "LS_TS")
		sortOrder = append(sortOrder, "LS_RTS")
		sortOrder = append(sortOrder, "R")
	default:
		logger.Fatal("unknown sort type", zap.String("get", cmdFlags.Common.SortOrder))
	}

	conf := confS{
		blockDir:           cmdFlags.PromBlock.Dir,
		blockName:          cmdFlags.PromBlock.Name,
		outDir:             cmdFlags.Out.Dir,
		sortOrder:          sortOrder,
		scaleFactor:        cmdFlags.Common.ScaleFactor,
		whiteJobLabelValue: whiteJobLabelValue,
		pageSize:           16777216, // 16Mb
	}
	return conf
}

type sampleStructLS struct {
	tsDelta int32
	value   float64
}

type sample struct {
	id      int32
	tsDelta int32
	value   float64
}

type sampleStructTS struct {
	id    int32
	value float64
}

func getSortedSamples(
	logger *zap.Logger,
	block *tsdb.Block,
	series *[]seriesStruct,
	firstTS int64,
	scaleFactor int,
) (
	tsIndex []int32,
	lsIndex []int32,
	sByTS map[int32][]sampleStructTS,
	sByLS map[int32][]sampleStructLS,
) {
	chksReader, err := block.Chunks()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	tsIndex = make([]int32, 0)
	lsIndex = make([]int32, 0)
	sByTS = make(map[int32][]sampleStructTS)
	sByLS = make(map[int32][]sampleStructLS)
	seriesLen := len(*series) - 1
	seriesIndex := make(map[uint64]struct{})
	extraSeries := make([]seriesStruct, 0)

	for i := range *series {
		extraLS := make([]labels.Labels, 0)
		extraLSHash := make([]uint64, 0)
		for j := 1; j < scaleFactor; j++ {
			lset := (*series)[i].ls
			lset = append(
				lset, labels.Label{
					Name:  fmt.Sprintf("exl%d", j),
					Value: fmt.Sprintf("exv%d", j),
				})
			extraLS = append(extraLS, lset)
			extraLSHash = append(extraLSHash, lset.Hash())
		}
		for _, chks := range (*series)[i].chunks {
			ch, errC := chksReader.Chunk(chks.Ref)
			if errC != nil {
				logger.Fatal("failed get chunk", zap.Error(errC))
			}
			it := ch.Iterator(nil)
			for it.Next() {
				ts, v := it.At()
				tsDelta := int32(ts - firstTS)

				if _, ok := sByTS[tsDelta]; !ok {
					tsIndex = append(tsIndex, tsDelta)
				}
				sByTS[tsDelta] = append(sByTS[tsDelta], sampleStructTS{id: int32(i), value: v})

				if _, ok := sByLS[int32(i)]; !ok {
					lsIndex = append(lsIndex, int32(i))
				}
				sByLS[int32(i)] = append(sByLS[int32(i)], sampleStructLS{value: v, tsDelta: tsDelta})

				if scaleFactor > 0 {
					for j := 1; j < scaleFactor; j++ {
						lsHash := extraLSHash[j-1]
						if _, ok := seriesIndex[lsHash]; !ok {
							extraSeries = append(extraSeries, seriesStruct{
								chunks: nil,
								lsByte: genByteFromLs(logger, extraLS[j-1]),
								s:      extraLS[j-1].String(),
							})
							seriesIndex[lsHash] = struct{}{}
							seriesLen++
						}

						if _, ok := sByTS[tsDelta]; !ok {
							tsIndex = append(tsIndex, tsDelta)
						}
						sByTS[tsDelta] = append(sByTS[tsDelta], sampleStructTS{id: int32(seriesLen), value: v + float64(j)})

						if _, ok := sByLS[int32(seriesLen)]; !ok {
							lsIndex = append(lsIndex, int32(seriesLen))
						}
						sByLS[int32(seriesLen)] = append(
							sByLS[int32(seriesLen)],
							sampleStructLS{
								value:   v + float64(j),
								tsDelta: tsDelta,
							},
						)
					}
				}
			}
			if errI := it.Err(); errI != nil {
				logger.Fatal("iterate by chunk failed", zap.Error(errI))
			}
		}
		(*series)[i].ls = nil
		(*series)[i].chunks = nil
	}

	*series = append(*series, extraSeries...)
	return tsIndex, lsIndex, sByTS, sByLS
}

type seriesStruct struct {
	ls     labels.Labels
	chunks []chunks.Meta
	lsByte []byte
	s      string
}

func getLabelsSets(
	logger *zap.Logger,
	block *tsdb.Block,
	indexReader tsdb.IndexReader,
	whiteJobLabelValue map[string]struct{},
) ([]seriesStruct, error) {
	seriesRefs := getSeriesRef(block, indexReader)

	var series []seriesStruct
	seriesIndex := make(map[uint64]struct{})
	var skipLabelset bool
	for s := range seriesRefs {
		skipLabelset = false
		var lset labels.Labels
		var chks []chunks.Meta
		err := indexReader.Series(s, &lset, &chks)
		if err != nil {
			return nil, err
		}
		// if exist white list for value of job label, skip label sets not from white list
		if len(whiteJobLabelValue) > 0 {
			for i := range lset {
				if _, ok := whiteJobLabelValue[lset[i].Value]; !ok && lset[i].Name == "job" {
					skipLabelset = true
				}
			}
		}

		if _, ok := seriesIndex[lset.Hash()]; !ok && !skipLabelset {
			series = append(series, seriesStruct{chunks: chks, ls: lset, lsByte: genByteFromLs(logger, lset), s: lset.String()})
			seriesIndex[lset.Hash()] = struct{}{}
		}
	}
	sort.Slice(series, func(i, j int) bool { return string(series[i].lsByte) < string(series[j].lsByte) })
	return series, nil
}

func getSeriesRef(block *tsdb.Block, indexReader tsdb.IndexReader) map[storage.SeriesRef]struct{} {
	lns, _ := block.LabelNames()
	seriesRefs := make(map[storage.SeriesRef]struct{})
	for _, ln := range lns {
		lvs, _ := indexReader.LabelValues(ln)

		for _, lv := range lvs {
			postingReader, errP := indexReader.Postings(ln, lv)
			if errP != nil {
				fmt.Println(errP)
				os.Exit(1)
			}
			postingIndex := indexReader.SortedPostings(postingReader)
			for postingIndex.Next() {
				s := postingIndex.At()
				seriesRefs[s] = struct{}{}
			}
			if errI := postingIndex.Err(); errI != nil {
				fmt.Println(errI)
				os.Exit(1)
			}
		}
	}
	return seriesRefs
}

func intToByte(logger *zap.Logger, i interface{}) []byte {
	var buf bytes.Buffer
	err := binary.Write(&buf, binary.LittleEndian, i)
	if err != nil {
		logger.Panic("binary.Write failed:", zap.Error(err))
	}
	return buf.Bytes()
}

func float64ToByte(f float64) []byte {
	var buf [8]byte
	n := math.Float64bits(f)
	buf[0] = byte(n >> 56)
	buf[1] = byte(n >> 48)
	buf[2] = byte(n >> 40)
	buf[3] = byte(n >> 32)
	buf[4] = byte(n >> 24)
	buf[5] = byte(n >> 16)
	buf[6] = byte(n >> 8)
	buf[7] = byte(n)
	return buf[:]
}

func genByteFromLs(logger *zap.Logger, ls labels.Labels) []byte {
	var buf []byte
	lsLen := int16(ls.Len()) * 2
	buf = append(buf, intToByte(logger, lsLen)...)

	var lName, lValue, symbolsLen int16
	for _, l := range ls {
		lName = int16(len(l.Name))
		lValue = int16(len(l.Value))
		buf = append(buf, intToByte(logger, lName)...)
		buf = append(buf, intToByte(logger, lValue)...)
		symbolsLen += lName + lValue
	}
	buf = append(buf, intToByte(logger, symbolsLen)...)

	for _, l := range ls {
		buf = append(buf, []byte(l.Name)...)
		buf = append(buf, []byte(l.Value)...)
	}
	bufLen := len(buf) % 8
	if bufLen != 0 {
		buf = append(buf, make([]byte, 8-bufLen)...)
	}
	return buf
}

func writeSampleSortedByTs(
	logger *zap.Logger,
	pageSize int,
	outPutPath string,
	series []seriesStruct,
	tsIndex []int32,
	sByTS map[int32][]sampleStructTS,
	firstTS int64,
) int64 {
	f, err := os.Create(outPutPath) //nolint:gosec // control by settings
	if err != nil {
		logger.Fatal("failed create output file", zap.Error(err))
	}
	defer func() {
		errC := f.Close()
		if errC != nil {
			logger.Fatal("failed close output file", zap.Error(errC))
		}
	}()

	// set buffer size 16 Mb
	w := bufio.NewWriterSize(f, 1*1000000*16)
	defer func() {
		errF := w.Flush()
		if errF != nil {
			logger.Fatal("failed flush write buffer", zap.Error(errF))
		}
	}()

	zw := lz4.NewWriter(w)
	defer func() {
		errF := zw.Flush()
		if errF != nil {
			logger.Fatal("failed flush lz4 to file", zap.Error(errF))
		}
		errC := zw.Close()
		if errC != nil {
			logger.Fatal("failed close lz4 to file", zap.Error(errC))
		}
	}()

	byteBuilder := new(bytes.Buffer)
	gt := time.Now()

	var samplesCount, totalSamplesCount int64
	for _, deltaTs := range tsIndex {
		tsInByte := intToByte(logger, int64(deltaTs)+firstTS)
		for i := range sByTS[deltaTs] {
			if time.Since(gt) >= time.Minute {
				logger.Info("convert data", zap.Int64("samples converted", totalSamplesCount))
				gt = time.Now()
			}

			valueBytes := float64ToByte(sByTS[deltaTs][i].value)
			sampleByteLen := len(tsInByte) + len(valueBytes) + len(series[sByTS[deltaTs][i].id].lsByte)
			if byteBuilder.Len()+sampleByteLen+8 > pageSize {
				page := append(intToByte(logger, samplesCount), make([]byte, 0)...)
				page = append(page, byteBuilder.Bytes()...)
				if len(page)%pageSize != 0 {
					page = append(page, make([]byte, pageSize-len(page))...)
				}
				if _, errZW := zw.Write(page); errZW != nil {
					logger.Fatal("failed write to lz4 stream", zap.Error(errZW))
				}
				byteBuilder.Reset()
				samplesCount = 0
			}

			byteBuilder.Write(tsInByte)
			byteBuilder.Write(valueBytes)
			byteBuilder.Write(series[sByTS[deltaTs][i].id].lsByte)
			samplesCount++
			totalSamplesCount++
		}
	}

	if samplesCount > 0 {
		page := append(intToByte(logger, samplesCount), byteBuilder.Bytes()...)
		if _, errZW := zw.Write(page); errZW != nil {
			logger.Fatal("failed write to lz4 stream", zap.Error(errZW))
		}
	}
	return totalSamplesCount
}

func writeSampleSortedByLs(
	logger *zap.Logger,
	pageSize int,
	outPutPath string,
	series []seriesStruct,
	lsIndex []int32,
	sByLS map[int32][]sampleStructLS,
	firstTS int64,
) int64 {
	f, err := os.Create(outPutPath) //nolint:gosec // control by settings
	if err != nil {
		logger.Fatal("failed create output file", zap.Error(err))
	}
	defer func() {
		errC := f.Close()
		if errC != nil {
			logger.Fatal("failed close output file", zap.Error(errC))
		}
	}()

	// set buffer size 16 Mb
	w := bufio.NewWriterSize(f, 1*1000000*16)
	defer func() {
		errF := w.Flush()
		if errF != nil {
			logger.Fatal("failed flush write buffer", zap.Error(errF))
		}
	}()

	zw := lz4.NewWriter(w)
	defer func() {
		errF := zw.Flush()
		if errF != nil {
			logger.Fatal("failed flush lz4 to file", zap.Error(errF))
		}
		errC := zw.Close()
		if errC != nil {
			logger.Fatal("failed close lz4 to file", zap.Error(errC))
		}
	}()

	byteBuilder := new(bytes.Buffer)
	gt := time.Now()
	var samplesCount, totalSamplesCount int64
	for _, id := range lsIndex {
		for i := range sByLS[id] {
			if time.Since(gt) >= time.Minute {
				logger.Info("convert data", zap.Int64("samples converted", totalSamplesCount))
				gt = time.Now()
			}
			tsInByte := intToByte(logger, int64(sByLS[id][i].tsDelta)+firstTS)
			valueBytes := float64ToByte(sByLS[id][i].value)
			sampleByteLen := len(tsInByte) + len(valueBytes) + len(series[id].lsByte)
			if byteBuilder.Len()+sampleByteLen+8 > pageSize {
				page := append(intToByte(logger, samplesCount), make([]byte, 0)...)
				page = append(page, byteBuilder.Bytes()...)
				if len(page)%pageSize != 0 {
					page = append(page, make([]byte, pageSize-len(page))...)
				}
				if _, errZW := zw.Write(page); errZW != nil {
					logger.Fatal("failed write to lz4 stream", zap.Error(errZW))
				}
				byteBuilder.Reset()
				samplesCount = 0
			}

			byteBuilder.Write(tsInByte)
			byteBuilder.Write(valueBytes)
			byteBuilder.Write(series[id].lsByte)
			samplesCount++
			totalSamplesCount++
		}
	}
	if samplesCount > 0 {
		page := append(intToByte(logger, samplesCount), byteBuilder.Bytes()...)
		if _, errZW := zw.Write(page); errZW != nil {
			logger.Fatal("failed write to lz4 stream", zap.Error(errZW))
		}
	}
	return totalSamplesCount
}

func writeSamples(
	logger *zap.Logger,
	pageSize int,
	outPutPath string,
	series []seriesStruct,
	samples []sample,
	firstTS int64,
) int64 {
	f, err := os.Create(outPutPath) //nolint:gosec // control by settings
	if err != nil {
		logger.Fatal("failed create output file", zap.Error(err))
	}
	defer func() {
		errC := f.Close()
		if errC != nil {
			logger.Fatal("failed close output file", zap.Error(errC))
		}
	}()

	// set buffer size 16 Mb
	w := bufio.NewWriterSize(f, 1*1000000*16)
	defer func() {
		errF := w.Flush()
		if errF != nil {
			logger.Fatal("failed flush write buffer", zap.Error(errF))
		}
	}()

	zw := lz4.NewWriter(w)
	defer func() {
		errF := zw.Flush()
		if errF != nil {
			logger.Fatal("failed flush lz4 to file", zap.Error(errF))
		}
		errC := zw.Close()
		if errC != nil {
			logger.Fatal("failed close lz4 to file", zap.Error(errC))
		}
	}()

	byteBuilder := new(bytes.Buffer)
	gt := time.Now()

	var samplesCount, totalSamplesCount int64
	for i := range samples {
		if time.Since(gt) >= time.Minute {
			logger.Info("convert data", zap.Int64("samples converted", totalSamplesCount))
			gt = time.Now()
		}
		tsInByte := intToByte(logger, int64(samples[i].tsDelta)+firstTS)
		valueBytes := float64ToByte(samples[i].value)
		sampleByteLen := len(tsInByte) + len(valueBytes) + len(series[samples[i].id].lsByte)
		if byteBuilder.Len()+sampleByteLen+8 > pageSize {
			page := append(intToByte(logger, samplesCount), make([]byte, 0)...)
			page = append(page, byteBuilder.Bytes()...)
			if len(page)%pageSize != 0 {
				page = append(page, make([]byte, pageSize-len(page))...)
			}
			if _, errZW := zw.Write(page); errZW != nil {
				logger.Fatal("failed write to lz4 stream", zap.Error(errZW))
			}
			byteBuilder.Reset()
			samplesCount = 0
		}
		byteBuilder.Write(tsInByte)
		byteBuilder.Write(valueBytes)
		byteBuilder.Write(series[samples[i].id].lsByte)
		samplesCount++
		totalSamplesCount++
	}
	if samplesCount > 0 {
		page := append(intToByte(logger, samplesCount), byteBuilder.Bytes()...)
		if _, errZW := zw.Write(page); errZW != nil {
			logger.Fatal("failed write to lz4 stream", zap.Error(errZW))
		}
	}
	return totalSamplesCount
}
