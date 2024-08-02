package cppbridge_test

import (
	"context"
	"fmt"
	"testing"
	"time"
	"unsafe"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
)

type RelabelerSuite struct {
	suite.Suite
	baseCtx context.Context
}

func TestRelabelerSuite(t *testing.T) {
	suite.Run(t, new(RelabelerSuite))
}

func (s *RelabelerSuite) SetupTest() {
	s.baseCtx = context.Background()
}

func (s *RelabelerSuite) TestRelabelConfigValidate() {
	tests := []struct {
		expected string
		config   cppbridge.RelabelConfig
	}{
		{
			config:   cppbridge.RelabelConfig{},
			expected: `relabel action cannot be empty`,
		},
		{
			config: cppbridge.RelabelConfig{
				Action: cppbridge.Replace,
			},
			expected: `requires 'target_label' value`,
		},
		{
			config: cppbridge.RelabelConfig{
				Action: cppbridge.Lowercase,
			},
			expected: `requires 'target_label' value`,
		},
		{
			config: cppbridge.RelabelConfig{
				Action:      cppbridge.Lowercase,
				Replacement: "$1",
				TargetLabel: "${3}",
			},
			expected: `"${3}" is invalid 'target_label'`,
		},
		{
			config: cppbridge.RelabelConfig{
				SourceLabels: []string{"a"},
				Regex:        "some-([^-]+)-([^,]+)",
				Action:       cppbridge.Replace,
				Replacement:  "${1}",
				TargetLabel:  "${3}",
			},
		},
		{
			config: cppbridge.RelabelConfig{
				SourceLabels: []string{"a"},
				Regex:        "some-([^-]+)-([^,]+)",
				Action:       cppbridge.Replace,
				Replacement:  "${1}",
				TargetLabel:  "0${3}",
			},
			expected: `"0${3}" is invalid 'target_label'`,
		},
		{
			config: cppbridge.RelabelConfig{
				SourceLabels: []string{"a"},
				Regex:        "some-([^-]+)-([^,]+)",
				Action:       cppbridge.Replace,
				Replacement:  "${1}",
				TargetLabel:  "-${3}",
			},
			expected: `"-${3}" is invalid 'target_label' for replace action`,
		},
	}
	for i, test := range tests {
		s.Run(fmt.Sprint(i), func() {
			err := test.config.Validate()
			if test.expected == "" {
				s.Require().NoError(err)
			} else {
				s.Require().ErrorContains(err, test.expected)
			}
		})
	}
}

func (s *RelabelerSuite) TestTargetLabelValidity() {
	tests := []struct {
		str   string
		valid bool
	}{
		{"-label", false},
		{"label", true},
		{"label${1}", true},
		{"${1}label", true},
		{"${1}", true},
		{"${1}label", true},
		{"${", false},
		{"$", false},
		{"${}", false},
		{"foo${", false},
		{"$1", true},
		{"asd$2asd", true},
		{"-foo${1}bar-", false},
		{"_${1}_", true},
		{"foo${bar}foo", true},
	}
	for _, test := range tests {
		s.Require().Equal(test.valid, cppbridge.RelabelTarget.MatchString(test.str),
			"Expected %q to be %v", test.str, test.valid)
	}
}

func (s *RelabelerSuite) TestAction() {
	raw := `action: Labelkeep`

	c := struct {
		Action cppbridge.Action `yaml:"action"`
	}{}

	err := yaml.Unmarshal([]byte(raw), &c)
	s.Require().NoError(err)

	s.Require().Equal(cppbridge.LabelKeep, c.Action)
}

func (s *RelabelerSuite) TestInvalidAction() {
	wr := prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	data, err := wr.Marshal()
	s.Require().NoError(err)

	rCfgs := []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"job"},
			Regex:        "abc",
			Action:       cppbridge.Action(20),
		},
	}

	lss := cppbridge.NewQueryableLssStorage()

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(rCfgs)
	s.Require().NoError(err)

	var numberOfShards uint16 = 1
	psr, err := cppbridge.NewInputPerShardRelabeler(statelessRelabeler, lss.Generation(), numberOfShards, 0)
	s.Require().NoError(err)

	hlimits := cppbridge.DefaultWALHashdexLimits()
	h, err := cppbridge.NewWALProtobufHashdex(data, hlimits)
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(numberOfShards)

	err = psr.InputRelabeling(s.baseCtx, lss, nil, h, shardsInnerSeries, shardsRelabeledSeries)
	s.Require().Error(err)
}

func (s *RelabelerSuite) TestPerShardRelabelerWithNullPtrStatelessRelabeler() {
	nilStatelessRelabeler := struct {
		p     uintptr
		rCfgs []*cppbridge.RelabelConfig
	}{0, nil}
	statelessRelabeler := (*cppbridge.StatelessRelabeler)(unsafe.Pointer(&nilStatelessRelabeler))

	_, err := cppbridge.NewInputPerShardRelabeler(statelessRelabeler, 0, 0, 0)
	s.Require().Error(err)
}

func (s *RelabelerSuite) TestInputPerShardRelabeler() {
	wr := prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	data, err := wr.Marshal()
	s.Require().NoError(err)

	rCfgs := []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"job"},
			Regex:        "abc",
			Action:       cppbridge.Keep,
		},
	}

	lss := cppbridge.NewQueryableLssStorage()

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(rCfgs)
	s.Require().NoError(err)

	var numberOfShards uint16 = 1
	psr, err := cppbridge.NewInputPerShardRelabeler(statelessRelabeler, lss.Generation(), numberOfShards, 0)
	s.Require().NoError(err)

	hlimits := cppbridge.DefaultWALHashdexLimits()
	h, err := cppbridge.NewWALProtobufHashdex(data, hlimits)
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(numberOfShards)

	err = psr.InputRelabeling(s.baseCtx, lss, nil, h, shardsInnerSeries, shardsRelabeledSeries)
	s.Require().NoError(err)
}

func (s *RelabelerSuite) TestOutputPerShardRelabeler() {
	rCfgs := []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"job"},
			Regex:        "abc",
			Action:       cppbridge.Keep,
		},
	}

	lss := cppbridge.NewQueryableLssStorage()

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(rCfgs)
	s.Require().NoError(err)

	externalLabels := []cppbridge.Label{
		{"name0", "value0"},
		{"name1", "value1"},
	}

	psr, err := cppbridge.NewOutputPerShardRelabeler(externalLabels, statelessRelabeler, lss.Generation(), 0, 0)
	s.Require().NoError(err)

	psr.CacheAllocatedMemory()
}

func (s *RelabelerSuite) TestRelabelerStateUpdateCtor() {
	var generation uint32 = 0

	rsu := cppbridge.NewRelabelerStateUpdate()
	s.Equal(generation, rsu.Generation())
}

func (s *RelabelerSuite) TestRelabelerStateUpdateWithGenerationCtor() {
	var generation uint32 = 3

	rsu := cppbridge.NewRelabelerStateUpdateWithGeneration(generation)
	s.Equal(generation, rsu.Generation())
}
