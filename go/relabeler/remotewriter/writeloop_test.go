package remotewriter

import (
	"context"
	"errors"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/manager"
	"github.com/prometheus/client_golang/prometheus"
	config3 "github.com/prometheus/common/config"
	config2 "github.com/prometheus/prometheus/config"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
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

type TestHeads struct {
	Dir            string
	clock          clockwork.Clock
	FileLog        *catalog.FileLog
	Catalog        *catalog.Catalog
	ConfigSource   *ConfigSource
	Manager        *manager.Manager
	NumberOfShards uint8
	Head           relabeler.Head
}

func NewTestHeads(dir string, inputRelabelConfigs []*config.InputRelabelerConfig, numberOfShards uint16, clock clockwork.Clock) (*TestHeads, error) {
	th := &TestHeads{
		Dir: dir,
	}
	var err error
	th.FileLog, err = catalog.NewFileLog(filepath.Join(dir, "catalog"), catalog.DefaultEncoder{}, catalog.DefaultDecoder{})
	if err != nil {
		return nil, err
	}

	th.Catalog, err = catalog.New(clock, th.FileLog)
	if err != nil {
		return nil, errors.Join(err, th.Close())
	}

	th.ConfigSource = &ConfigSource{
		inputRelabelConfigs: inputRelabelConfigs,
		numberOfShards:      numberOfShards,
	}

	th.Manager, err = manager.New(dir, th.ConfigSource, th.Catalog, prometheus.DefaultRegisterer)
	if err != nil {
		return nil, errors.Join(err, th.Close())
	}

	th.Head, err = th.Manager.Build()
	if err != nil {
		return nil, errors.Join(err, th.Close())
	}

	return th, nil
}

func (th *TestHeads) Append(ctx context.Context, timeSeriesSlice []model.TimeSeries, relabelerID string) error {
	hx, err := cppbridge.HashdexFactory{}.GoModel(timeSeriesSlice, cppbridge.DefaultWALHashdexLimits())
	if err != nil {
		return err
	}

	_, err = th.Head.Append(ctx, &relabeler.IncomingData{Hashdex: hx}, nil, relabelerID)
	return err
}

func (th *TestHeads) Rotate() error {
	cfg, nos := th.ConfigSource.Get()
	head, err := th.Manager.BuildWithConfig(cfg, nos)
	if err != nil {
		return err
	}
	th.Head.Finalize()
	if err = th.Head.Close(); err != nil {
		return err
	}
	th.Head = head
	return nil
}

func (th *TestHeads) Close() error {
	return errors.Join(th.Head.Close(), th.FileLog.Close())
}

func TestWriteLoopIterate(t *testing.T) {
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

	batches := [][]model.TimeSeries{
		{
			{LabelSet: labelSets[0], Timestamp: 0, Value: 0},
			{LabelSet: labelSets[1], Timestamp: 0, Value: 1000},
			{LabelSet: labelSets[2], Timestamp: 0, Value: 1000000},
			{LabelSet: labelSets[3], Timestamp: 0, Value: 1000000000},
		},
	}

	err = testHeads.Append(ctx, batches[0], transparentRelabelerName)
	require.NoError(t, err)

	destination := NewDestination(DestinationConfig{
		RemoteWriteConfig: config2.RemoteWriteConfig{
			URL:                  nil,
			RemoteTimeout:        0,
			Headers:              nil,
			WriteRelabelConfigs:  nil,
			Name:                 "remote_write_0",
			SendExemplars:        false,
			SendNativeHistograms: false,
			HTTPClientConfig:     config3.HTTPClientConfig{},
			QueueConfig:          config2.QueueConfig{},
			MetadataConfig:       config2.MetadataConfig{},
			SigV4Config:          nil,
			AzureADConfig:        nil,
		},
	})
	wl := newWriteLoop(destination, testHeads.Catalog)
	ds, err := wl.nextDataSource()
	require.NoError(t, err)

	err = wl.iterate(ctx, ds)
}
