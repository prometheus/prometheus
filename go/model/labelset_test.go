package model_test

import (
	"encoding/json"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/pp/go/model"
)

type LabelSetSuite struct {
	suite.Suite
}

func TestLabelSet(t *testing.T) {
	suite.Run(t, new(LabelSetSuite))
}

func (s *LabelSetSuite) TestLabelSet_FromSlice() {
	ls := model.LabelSetFromSlice([]model.SimpleLabel{
		{"__name__", "example"},
		{"instance", "instance"},
		{"job", "test"},
		{"container", "~unknown"},
		{"flags", "empty"},
	})
	s.Equal("__name__:example;container:~unknown;flags:empty;instance:instance;job:test;", ls.String())
}

func (s *LabelSetSuite) TestLabelSet_FromMap() {
	ls := model.LabelSetFromMap(map[string]string{
		"__name__":  "example",
		"instance":  "instance",
		"job":       "test",
		"container": "~unknown",
		"flags":     "empty",
	})
	s.Equal("__name__:example;container:~unknown;flags:empty;instance:instance;job:test;", ls.String())
}

func (s *LabelSetSuite) TestLabelSet_FromPairs() {
	ls := model.LabelSetFromPairs(
		"__name__", "example",
		"instance", "instance",
		"job", "test",
		"container", "~unknown",
		"flags", "empty",
	)
	s.Equal("__name__:example;container:~unknown;flags:empty;instance:instance;job:test;", ls.String())
}

func (s *LabelSetSuite) TestLabelSet_FromPairs_panic() {
	s.Panics(func() {
		model.LabelSetFromPairs(
			"__name__", "example",
			"instance", "instance",
			"job", "test",
			"container", "~unknown",
			"flags", // "empty",
		)
	})
}

func (s *LabelSetSuite) TestLabelSet_Get() {
	ls := model.LabelSetFromMap(map[string]string{
		"__name__":  "example",
		"instance":  "instance",
		"job":       "test",
		"container": "~unknown",
		"flags":     "empty",
	})
	s.Equal("test", ls.Get("job", ""))
	s.Equal("not found", ls.Get("plugin", "not found"))
}

func (s *LabelSetSuite) TestLabelSet_With() {
	ls := model.LabelSetFromPairs(
		"__name__", "example",
		"instance", "instance",
		"job", "test",
		"container", "~unknown",
		"flags", "empty",
	)
	s.Equal(
		"__name__:example;container:~unknown;flags:empty;instance:instance;job:test;",
		ls.With("flags", "empty").String())
	s.Equal(
		"__name__:example;container:service;flags:empty;instance:instance;job:test;",
		ls.With("container", "service").String())
	s.Equal(
		"__name__:example;container:~unknown;flags:empty;image:added;instance:instance;job:test;",
		ls.With("image", "added").String())
}

func (s *LabelSetSuite) TestLabelSet_WithPairs() {
	ls := model.LabelSetFromPairs(
		"__name__", "example",
		"instance", "instance",
		"job", "test",
		"container", "~unknown",
		"flags", "empty",
	)
	merged := ls.WithPairs(
		"flags", "empty",
		"container", "service",
		"image", "added",
	)
	s.Equal(
		"__name__:example;container:service;flags:empty;image:added;instance:instance;job:test;",
		merged.String())
}

func (s *LabelSetSuite) TestLabelSet_Merge() {
	ls := model.LabelSetFromPairs(
		"__name__", "example",
		"instance", "instance",
		"job", "test",
		"container", "~unknown",
		"flags", "empty",
	)
	merged := ls.Merge(model.LabelSetFromPairs(
		"flags", "empty",
		"container", "service",
		"image", "added",
	))
	s.Equal(
		"__name__:example;container:service;flags:empty;image:added;instance:instance;job:test;",
		merged.String())
}

func (s *LabelSetSuite) TestLabelSet_Merge_empty() {
	ls := model.LabelSetFromPairs(
		"__name__", "example",
		"instance", "instance",
		"job", "test",
		"container", "~unknown",
		"flags", "empty",
	)
	merged := ls.Merge(model.EmptyLabelSet())
	s.Equal(
		"__name__:example;container:~unknown;flags:empty;instance:instance;job:test;",
		merged.String())
}

func (s *LabelSetSuite) TestLabelSet_Split() {
	ls := model.LabelSetFromPairs(
		"__name__", "example",
		"instance", "instance",
		"job", "test",
		"container", "~unknown",
		"flags", "empty",
	)
	extracted, rest := ls.SplitBy("flags", "container", "image")
	s.Equal("container:~unknown;flags:empty;", extracted.String())
	s.Equal("__name__:example;instance:instance;job:test;", rest.String())
}

func (s *LabelSetSuite) TestLabelSet_Map_Quick() {
	identity := func(m map[string]string) map[string]string { return m }
	convertation := func(m map[string]string) map[string]string {
		return model.LabelSetFromMap(m).ToMap()
	}
	s.Require().NoError(quick.CheckEqual(identity, convertation, nil))
}

func (s *LabelSetSuite) TestLabelSet_MarshalJSON_Quick() {
	identity := func(m map[string]string) map[string]string { return m }
	convertation := func(m map[string]string) map[string]string {
		ls := model.LabelSetFromMap(m)
		blob, err := json.Marshal(ls)
		s.Require().NoError(err)
		res := map[string]string{}
		err = json.Unmarshal(blob, &res)
		s.Require().NoError(err)
		return res
	}
	s.Require().NoError(quick.CheckEqual(identity, convertation, nil))
}

func (s *LabelSetSuite) TestLabelSet_UnmarshalJSON_Quick() {
	identity := func(m map[string]string) map[string]string { return m }
	convertation := func(m map[string]string) map[string]string {
		blob, err := json.Marshal(m)
		s.Require().NoError(err)
		var ls model.LabelSet
		err = json.Unmarshal(blob, &ls)
		s.Require().NoError(err)
		return ls.ToMap()
	}
	s.Require().NoError(quick.CheckEqual(identity, convertation, nil))
}

func (s *LabelSetSuite) TestLabelSet_MarshalYAML_Quick() {
	identity := func(m map[string]string) map[string]string { return m }
	convertation := func(m map[string]string) map[string]string {
		ls := model.LabelSetFromMap(m)
		blob, err := yaml.Marshal(ls)
		s.Require().NoError(err)
		res := map[string]string{}
		err = yaml.Unmarshal(blob, &res)
		s.Require().NoError(err)
		return res
	}
	s.Require().NoError(quick.CheckEqual(identity, convertation, nil))
}

func (s *LabelSetSuite) TestLabelSet_UnmarshalYAML_Quick() {
	identity := func(m map[string]string) map[string]string { return m }
	convertation := func(m map[string]string) map[string]string {
		blob, err := yaml.Marshal(m)
		s.Require().NoError(err)
		var ls model.LabelSet
		err = yaml.Unmarshal(blob, &ls)
		s.Require().NoError(err)
		return ls.ToMap()
	}
	s.Require().NoError(quick.CheckEqual(identity, convertation, nil))
}

func (s *LabelSetSuite) TestLabelSetSimpleBuilder_Build() {
	labels := []model.SimpleLabel{
		{"__name__", "example"},
		{"container", "~unknown"},
		{"instance", "instance"},
		{"job", "test"},
		{"flags", "empty"},
	}

	builder := model.NewLabelSetSimpleBuilder()
	for _, l := range labels {
		builder.Add(l.Name, l.Value)
	}
	ls := builder.Build()
	s.Equal("__name__:example;container:~unknown;flags:empty;instance:instance;job:test;", ls.String())
}

func (s *LabelSetSuite) TestLabelSetSimpleBuilder_Get() {
	labels := []model.SimpleLabel{
		{"__name__", "example"},
		{"container", "~unknown"},
		{"instance", "instance"},
		{"job", "test"},
		{"flags", "empty"},
	}

	builder := model.NewLabelSetSimpleBuilderSize(len(labels))
	for _, l := range labels {
		builder.Add(l.Name, l.Value)
	}

	s.Equal("~unknown", builder.Get("container"))
	s.Equal("", builder.Get("container2"))
}

func (s *LabelSetSuite) TestLabelSetSimpleBuilder_Has() {
	labels := []model.SimpleLabel{
		{"__name__", "example"},
		{"container", "~unknown"},
		{"instance", "instance"},
		{"job", "test"},
		{"flags", "empty"},
	}

	builder := model.NewLabelSetSimpleBuilder()
	for _, l := range labels {
		builder.Add(l.Name, l.Value)
	}

	s.True(builder.Has("container"))
	s.False(builder.Has("container2"))
}

func (s *LabelSetSuite) TestLabelSetSimpleBuilder_HasDuplicateLabelNames() {
	labels := []model.SimpleLabel{
		{"__name__", "example"},
		{"container", "~unknown"},
		{"instance", "instance"},
		{"job", "test"},
		{"container", "empty"},
	}

	builder := model.NewLabelSetSimpleBuilder()
	for _, l := range labels {
		builder.Add(l.Name, l.Value)
	}

	dup, has := builder.HasDuplicateLabelNames()
	s.Equal("container", dup)
	s.True(has)
}

func (s *LabelSetSuite) TestLabelSetSimpleBuilder_HasNotDuplicateLabelNames() {
	labels := []model.SimpleLabel{
		{"__name__", "example"},
		{"container", "~unknown"},
		{"instance", "instance"},
		{"job", "test"},
		{"flags", "empty"},
	}

	builder := model.NewLabelSetSimpleBuilder()
	for _, l := range labels {
		builder.Add(l.Name, l.Value)
	}

	dup, has := builder.HasDuplicateLabelNames()
	s.Equal("", dup)
	s.False(has)
}

func (s *LabelSetSuite) TestLabelSetSimpleBuilder_HasNotDuplicateLabelNames_Set() {
	labels := []model.SimpleLabel{
		{"__name__", "example"},
		{"container", "~unknown"},
		{"instance", "instance"},
		{"job", "test"},
		{"container", "empty"},
	}

	builder := model.NewLabelSetSimpleBuilder()
	for _, l := range labels {
		builder.Set(l.Name, l.Value)
	}

	dup, has := builder.HasDuplicateLabelNames()
	s.Equal("", dup)
	s.False(has)
}

func (s *LabelSetSuite) TestLabelSetSimpleBuilder_Reset() {
	labels := []model.SimpleLabel{
		{"__name__", "example"},
		{"flags", "empty"},
		{"container", "~unknown"},
		{"instance", "instance"},
		{"job", "test"},
	}

	builder := model.NewLabelSetSimpleBuilder()
	for _, l := range labels {
		builder.Add(l.Name, l.Value)
	}
	ls := builder.Build()
	s.Equal("__name__:example;container:~unknown;flags:empty;instance:instance;job:test;", ls.String())

	builder.Reset()

	ls = builder.Build()
	s.Equal("", ls.String())
}

func (s *LabelSetSuite) TestLabelSetSimpleBuilder_Sort() {
	labels := []model.SimpleLabel{
		{"__name__", "example"},
		{"flags", "empty"},
		{"container", "~unknown"},
		{"instance", "instance"},
		{"job", "test"},
	}

	builder := model.NewLabelSetSimpleBuilder()
	for _, l := range labels {
		builder.Add(l.Name, l.Value)
	}
	builder.Sort()
	ls := builder.Build()
	builder.Sort()
	s.Equal("__name__:example;container:~unknown;flags:empty;instance:instance;job:test;", ls.String())
}

func BenchmarkDecoderV3(b *testing.B) {
	labels := []model.SimpleLabel{
		{"__name__", "example"},
		{"container", "~unknown"},
		{"instance", "instance"},
		{"job", "test"},
		{"flags", "empty"},
		{"flags1", "empty"},
	}

	builder := model.NewLabelSetSimpleBuilder()
	for _, l := range labels {
		builder.Add(l.Name, l.Value)
	}

	for i := 0; i < b.N; i++ {
		v := builder.Has("flagsq")
		if !v {
			//
		}

		// v := builder.Get("flagsq")
		// if v == "" {
		// 	//
		// }
	}
}

// BenchmarkDecoderV3-8   	12041396	       104.2 ns/op	       0 B/op	       0 allocs/op
