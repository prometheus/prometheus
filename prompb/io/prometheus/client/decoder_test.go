package io_prometheus_client

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

const (
	testGauge = `name: "go_build_info"
help: "Build information about the main Go module."
type: GAUGE
metric: <
  label: <
    name: "checksum"
    value: ""
  >
  label: <
    name: "path"
    value: "github.com/prometheus/client_golang"
  >
  label: <
    name: "version"
    value: "(devel)"
  >
  gauge: <
    value: 1
  >
>
metric: <
  label: <
    name: "checksum"
    value: ""
  >
  label: <
    name: "path"
    value: "github.com/prometheus/prometheus"
  >
  label: <
    name: "version"
    value: "v3.0.0"
  >
  gauge: <
    value: 2
  >
>

`
	testCounter = `name: "go_memstats_alloc_bytes_total"
help: "Total number of bytes allocated, even if freed."
type: COUNTER
unit: "bytes"
metric: <
  counter: <
    value: 1.546544e+06
    exemplar: <
      label: <
        name: "dummyID"
        value: "42"
      >
      value: 12
      timestamp: <
        seconds: 1625851151
        nanos: 233181499
      >
    >
  >
>

`
)

func TestMetricStreamingDecoder(t *testing.T) {
	varintBuf := make([]byte, binary.MaxVarintLen32)
	buf := bytes.Buffer{}
	for _, m := range []string{testGauge, testCounter} {
		mf := &MetricFamily{}
		require.NoError(t, proto.UnmarshalText(m, mf))
		// From proto message to binary protobuf.
		protoBuf, err := proto.Marshal(mf)
		require.NoError(t, err)

		// Write first length, then binary protobuf.
		varintLength := binary.PutUvarint(varintBuf, uint64(len(protoBuf))) //?
		buf.Write(varintBuf[:varintLength])
		buf.Write(protoBuf)
	}

	d := NewMetricStreamingDecoder(buf.Bytes())

	require.NoError(t, d.NextMetricFamily())
	nextFn := func() error {
		for {
			err := d.NextMetric()
			if err == io.EOF {
				if err := d.NextMetricFamily(); err != nil {
					return err
				}
				continue
			}
			return err
		}
	}

	var firstMetricLset labels.Labels
	{
		require.NoError(t, nextFn())

		require.Equal(t, "go_build_info", d.GetName())
		require.Equal(t, "Build information about the main Go module.", d.GetHelp())
		require.Equal(t, MetricType_GAUGE, d.GetType())

		require.Equal(t, float64(1), d.GetGauge().GetValue())
		b := labels.NewScratchBuilder(0)
		require.NoError(t, d.Label(&b))

		firstMetricLset = b.Labels()

		require.Equal(t, `{checksum="", path="github.com/prometheus/client_golang", version="(devel)"}`, firstMetricLset.String())
	}

	{
		require.NoError(t, nextFn())

		require.Equal(t, "go_build_info", d.GetName())
		require.Equal(t, "Build information about the main Go module.", d.GetHelp())
		require.Equal(t, MetricType_GAUGE, d.GetType())

		require.Equal(t, float64(2), d.GetGauge().GetValue())
		b := labels.NewScratchBuilder(0)
		require.NoError(t, d.Label(&b))
		require.Equal(t, `{checksum="", path="github.com/prometheus/prometheus", version="v3.0.0"}`, b.Labels().String())
	}
	{
		// Different mf now.
		require.NoError(t, nextFn())

		require.Equal(t, "go_memstats_alloc_bytes_total", d.GetName())
		require.Equal(t, "Total number of bytes allocated, even if freed.", d.GetHelp())
		require.Equal(t, "bytes", d.GetUnit())
		require.Equal(t, MetricType_COUNTER, d.GetType())

		require.Equal(t, 1.546544e+06, d.Metric.GetCounter().GetValue())
		b := labels.NewScratchBuilder(0)
		require.NoError(t, d.Label(&b))
		require.Equal(t, `{}`, b.Labels().String())
	}

	require.Equal(t, io.EOF, nextFn())

	// Expect labels and metricBytes to be static and reusable even after parsing.
	require.Equal(t, `{checksum="", path="github.com/prometheus/client_golang", version="(devel)"}`, firstMetricLset.String())

}
