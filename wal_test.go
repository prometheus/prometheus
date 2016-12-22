package tsdb

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/fabxc/tsdb/labels"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func BenchmarkWALWrite(b *testing.B) {
	d, err := ioutil.TempDir("", "wal_read_test")
	require.NoError(b, err)

	defer func() {
		require.NoError(b, os.RemoveAll(d))
	}()

	wal, err := OpenWAL(d)
	require.NoError(b, err)

	f, err := os.Open("cmd/tsdb/testdata.1m")
	require.NoError(b, err)

	series, err := readPrometheusLabels(f, b.N)
	require.NoError(b, err)

	var (
		samples [][]hashedSample
		ts      int64
	)
	for i := 0; i < 300; i++ {
		ts += int64(30000)
		scrape := make([]hashedSample, 0, len(series))

		for ref := range series {
			scrape = append(scrape, hashedSample{
				ref: uint32(ref),
				t:   ts,
				v:   12345788,
			})
		}
		samples = append(samples, scrape)
	}

	b.ResetTimer()

	err = wal.Log(series, samples[0])
	require.NoError(b, err)

	for _, s := range samples[1:] {
		err = wal.Log(nil, s)
		require.NoError(b, err)
	}

	require.NoError(b, wal.Close())
}

func BenchmarkWALRead(b *testing.B) {
	f, err := os.Open("cmd/tsdb/testdata.1m")
	require.NoError(b, err)

	series, err := readPrometheusLabels(f, 1000000)
	require.NoError(b, err)

	b.Run("test", func(b *testing.B) {
		bseries := series[:b.N]

		d, err := ioutil.TempDir("", "wal_read_test")
		require.NoError(b, err)

		defer func() {
			require.NoError(b, os.RemoveAll(d))
		}()

		wal, err := OpenWAL(d)
		require.NoError(b, err)

		var (
			samples [][]hashedSample
			ts      int64
		)
		for i := 0; i < 300; i++ {
			ts += int64(30000)
			scrape := make([]hashedSample, 0, len(bseries))

			for ref := range bseries {
				scrape = append(scrape, hashedSample{
					ref: uint32(ref),
					t:   ts,
					v:   12345788,
				})
			}
			samples = append(samples, scrape)
		}

		err = wal.Log(bseries, samples[0])
		require.NoError(b, err)

		for _, s := range samples[1:] {
			err = wal.Log(nil, s)
			require.NoError(b, err)
		}

		require.NoError(b, wal.Close())

		b.ResetTimer()

		wal, err = OpenWAL(d)
		require.NoError(b, err)

		var numSeries, numSamples int

		err = wal.ReadAll(&walHandler{
			series: func(lset labels.Labels) { numSeries++ },
			sample: func(smpl hashedSample) { numSamples++ },
		})
		require.NoError(b, err)

		stat, _ := wal.f.Stat()
		fmt.Println("read series", numSeries, "read samples", numSamples, "wal size", fmt.Sprintf("%.2fMiB", float64(stat.Size())/1024/1024))
	})
}

func BenchmarkWALReadIntoHead(b *testing.B) {
	f, err := os.Open("cmd/tsdb/testdata.1m")
	require.NoError(b, err)

	series, err := readPrometheusLabels(f, 1000000)
	require.NoError(b, err)

	b.Run("test", func(b *testing.B) {
		bseries := series[:b.N]

		d, err := ioutil.TempDir("", "wal_read_test")
		require.NoError(b, err)

		defer func() {
			require.NoError(b, os.RemoveAll(d))
		}()

		wal, err := OpenWAL(d)
		require.NoError(b, err)

		var (
			samples [][]hashedSample
			ts      int64
		)
		for i := 0; i < 300; i++ {
			ts += int64(30000)
			scrape := make([]hashedSample, 0, len(bseries))

			for ref := range bseries {
				scrape = append(scrape, hashedSample{
					ref: uint32(ref),
					t:   ts,
					v:   12345788,
				})
			}
			samples = append(samples, scrape)
		}

		err = wal.Log(bseries, samples[0])
		require.NoError(b, err)

		for _, s := range samples[1:] {
			err = wal.Log(nil, s)
			require.NoError(b, err)
		}

		require.NoError(b, wal.Close())

		b.ResetTimer()

		head, err := NewHeadBlock(d, 0)
		require.NoError(b, err)

		stat, _ := head.wal.f.Stat()
		fmt.Println("head block initialized from WAL")
		fmt.Println("read series", head.stats.SeriesCount, "read samples", head.stats.SampleCount, "wal size", fmt.Sprintf("%.2fMiB", float64(stat.Size())/1024/1024))
	})
}

func readPrometheusLabels(r io.Reader, n int) ([]labels.Labels, error) {
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

	lbls := make([]labels.Labels, 0, n)

	for _, m := range mets[:n] {
		lset := make(labels.Labels, 0, len(m))
		for k, v := range m {
			lset = append(lset, labels.Label{Name: string(k), Value: string(v)})
		}
		lbls = append(lbls, lset)
	}

	return lbls, nil
}
