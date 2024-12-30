package clientv1

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"github.com/prometheus/prometheus/model/labels"
)

func TestMetric_ParseLabels(t *testing.T) {
	const testGauge = `name: "go_build_info"
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

`
	var protoBuf []byte
	{
		mf := &MetricFamily{}
		require.NoError(t, prototext.Unmarshal([]byte(testGauge), mf))
		// From proto message to binary protobuf.
		var err error
		protoBuf, err = proto.Marshal(mf)
		require.NoError(t, err)
	}

	mf := &MetricFamily{}
	require.NoError(t, proto.Unmarshal(protoBuf, mf))
	m := mf.GetMetric()
	require.Len(t, m, 1)

	b := labels.NewScratchBuilder(0)
	require.NoError(t, m[0].ParseLabels(&b))
	require.Equal(t, `{checksum="", path="github.com/prometheus/client_golang", version="(devel)"}`, b.Labels().String())
}
