package remotewriter

import (
	"context"
	"errors"
	"github.com/golang/snappy"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/appender"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/manager"
	"github.com/prometheus/client_golang/prometheus"
	config3 "github.com/prometheus/common/config"
	model2 "github.com/prometheus/common/model"
	config2 "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const transparentRelabelerName = "transparent_relabeler"

type ConfigSource struct {
	inputRelabelConfigs []*config.InputRelabelerConfig
	numberOfShards      uint16
}

func (s *ConfigSource) Set(inputRelabelConfigs []*config.InputRelabelerConfig, numberOfShards uint16) {
	s.inputRelabelConfigs = inputRelabelConfigs
	s.numberOfShards = numberOfShards
}

func (s *ConfigSource) Get() (inputRelabelConfigs []*config.InputRelabelerConfig, numberOfShards uint16) {
	return s.inputRelabelConfigs, s.numberOfShards
}

type NoOpStorage struct {
	heads []relabeler.Head
}

func (s *NoOpStorage) Add(head relabeler.Head) {
	s.heads = append(s.heads, head)
}

func (s *NoOpStorage) Close() error {
	var err error
	for _, head := range s.heads {
		err = errors.Join(err, head.Close())
	}
	s.heads = nil
	return err
}

type TestHeads struct {
	Dir            string
	clock          clockwork.Clock
	FileLog        *catalog.FileLog
	Catalog        *catalog.Catalog
	ConfigSource   *ConfigSource
	Manager        *manager.Manager
	NumberOfShards uint8
	Storage        *NoOpStorage
	Head           relabeler.Head
}

func NewTestHeads(dir string, inputRelabelConfigs []*config.InputRelabelerConfig, numberOfShards uint16, clock clockwork.Clock) (*TestHeads, error) {
	th := &TestHeads{
		Dir: dir,
	}
	var err error
	th.FileLog, err = catalog.NewFileLogV2(filepath.Join(dir, "catalog"))
	if err != nil {
		return nil, err
	}

	th.Catalog, err = catalog.New(clock, th.FileLog, catalog.DefaultIDGenerator{})
	if err != nil {
		return nil, errors.Join(err, th.Close())
	}

	th.ConfigSource = &ConfigSource{
		inputRelabelConfigs: inputRelabelConfigs,
		numberOfShards:      numberOfShards,
	}

	th.Manager, err = manager.New(dir, clock, th.ConfigSource, th.Catalog, 0, prometheus.DefaultRegisterer)
	if err != nil {
		return nil, errors.Join(err, th.Close())
	}

	activeHead, _, err := th.Manager.Restore(time.Hour)
	if err != nil {
		return nil, errors.Join(err, th.Close())
	}

	th.Storage = &NoOpStorage{}
	th.Head = appender.NewRotatableHead(activeHead, th.Storage, th.Manager, appender.NoOpHeadActivator{})

	return th, nil
}

func (th *TestHeads) Append(ctx context.Context, timeSeriesSlice []model.TimeSeries, relabelerID string) error {
	hx, err := cppbridge.HashdexFactory{}.GoModel(timeSeriesSlice, cppbridge.DefaultWALHashdexLimits())
	if err != nil {
		return err
	}

	_, _, err = th.Head.Append(ctx, &relabeler.IncomingData{Hashdex: hx}, nil, relabelerID, true)
	return err
}

func (th *TestHeads) Rotate() error {
	return th.Head.Rotate()
}

func (th *TestHeads) Close() error {
	return errors.Join(th.Storage.Close(), th.Head.Close(), th.FileLog.Close())
}

type remoteClient struct {
	mtx  sync.Mutex
	data [][]byte
	name string
}

func (c *remoteClient) Store(_ context.Context, bytes []byte, _ int) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.data = append(c.data, bytes)
	return nil
}

func (c *remoteClient) Name() string {
	return c.name
}

func (c *remoteClient) Endpoint() string {
	return ""
}

type testWriter struct {
	mtx  sync.Mutex
	data []*cppbridge.SnappyProtobufEncodedData
}

func (w *testWriter) Write(ctx context.Context, protobuf *cppbridge.SnappyProtobufEncodedData) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	w.data = append(w.data, protobuf)
	return ctx.Err()
}

func TestWriteLoopWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "write_loop_iterate_test")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	clock := clockwork.NewRealClock()
	cfgs := []*config.InputRelabelerConfig{
		{
			Name: transparentRelabelerName,
			RelabelConfigs: []*cppbridge.RelabelConfig{
				{
					SourceLabels: []string{"__name__"},
					Regex:        ".*",
					Action:       cppbridge.Keep,
				},
			},
		},
	}
	var numberOfShards uint16 = 2

	testHeads, err := NewTestHeads(tmpDir, cfgs, numberOfShards, clock)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	labelSets := []model.LabelSet{
		model.NewLabelSetBuilder().Set("__name__", "test_metric_0").Build(),
		model.NewLabelSetBuilder().Set("__name__", "test_metric_1").Build(),
		model.NewLabelSetBuilder().Set("__name__", "test_metric_2").Build(),
		model.NewLabelSetBuilder().Set("__name__", "test_metric_3").Build(),
	}

	ts := clock.Now().UnixMilli()
	batches := [][]model.TimeSeries{
		{
			{LabelSet: labelSets[0], Timestamp: uint64(ts), Value: 0},
			{LabelSet: labelSets[1], Timestamp: uint64(ts), Value: 1000},
			{LabelSet: labelSets[2], Timestamp: uint64(ts), Value: 1000000},
			{LabelSet: labelSets[3], Timestamp: uint64(ts), Value: 1000000000},
		},
	}

	err = testHeads.Append(ctx, batches[0], transparentRelabelerName)
	require.NoError(t, err)

	u, err := url.Parse("http://localhost:8080")
	require.NoError(t, err)

	destination := NewDestination(DestinationConfig{
		RemoteWriteConfig: config2.RemoteWriteConfig{
			URL:                  &config3.URL{u},
			RemoteTimeout:        0,
			Headers:              nil,
			WriteRelabelConfigs:  nil,
			Name:                 "remote_write_0",
			SendExemplars:        false,
			SendNativeHistograms: false,
			HTTPClientConfig:     config3.HTTPClientConfig{},
			QueueConfig: config2.QueueConfig{
				MaxSamplesPerSend: 2,
				MinShards:         3,
				MaxShards:         5,
				SampleAgeLimit:    model2.Duration(time.Hour),
			},
			MetadataConfig: config2.MetadataConfig{},
			SigV4Config:    nil,
			AzureADConfig:  nil,
		},
		ExternalLabels: labels.Labels{
			{Name: "lol", Value: "kek"},
		},
		ReadTimeout: time.Second * 3,
	})

	wl := newWriteLoop(tmpDir, destination, testHeads.Catalog, clock)
	w := &testWriter{}
	i, err := wl.nextIterator(ctx, w)
	require.NoError(t, err)

	require.NoError(t, i.Next(ctx))
	require.NoError(t, err)

	require.NoError(t, testHeads.Rotate())

	require.ErrorIs(t, i.Next(ctx), ErrEndOfBlock)

	for _, data := range w.data {
		wr := prompb.WriteRequest{}
		err = data.Do(func(buf []byte) error {
			var decoded []byte
			decoded, err = snappy.Decode(nil, buf)
			if err != nil {
				return err
			}
			return wr.Unmarshal(decoded)
		})
		require.NoError(t, err)
		t.Log(wr.String())
	}
}
