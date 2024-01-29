package model_test

import (
	"encoding/json"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/pp/go/model"
)

func TestLabelSet_FromMap(t *testing.T) {
	ls := model.LabelSetFromMap(map[string]string{
		"__name__":  "example",
		"instance":  "instance",
		"job":       "test",
		"container": "~unknown",
		"flags":     "empty",
	})
	assert.Equal(t, "__name__:example;container:~unknown;flags:empty;instance:instance;job:test;", ls.String())
}

func TestLabelSet_FromPairs(t *testing.T) {
	ls := model.LabelSetFromPairs(
		"__name__", "example",
		"instance", "instance",
		"job", "test",
		"container", "~unknown",
		"flags", "empty",
	)
	assert.Equal(t, "__name__:example;container:~unknown;flags:empty;instance:instance;job:test;", ls.String())
}

func TestLabelSet_FromPairs_panic(t *testing.T) {
	assert.Panics(t, func() {
		model.LabelSetFromPairs(
			"__name__", "example",
			"instance", "instance",
			"job", "test",
			"container", "~unknown",
			"flags", // "empty",
		)
	})
}

func TestLabelSet_Get(t *testing.T) {
	ls := model.LabelSetFromMap(map[string]string{
		"__name__":  "example",
		"instance":  "instance",
		"job":       "test",
		"container": "~unknown",
		"flags":     "empty",
	})
	assert.Equal(t, "test", ls.Get("job", ""))
	assert.Equal(t, "not found", ls.Get("plugin", "not found"))
}

func TestLabelSet_With(t *testing.T) {
	ls := model.LabelSetFromPairs(
		"__name__", "example",
		"instance", "instance",
		"job", "test",
		"container", "~unknown",
		"flags", "empty",
	)
	assert.Equal(t,
		"__name__:example;container:~unknown;flags:empty;instance:instance;job:test;",
		ls.With("flags", "empty").String())
	assert.Equal(t,
		"__name__:example;container:service;flags:empty;instance:instance;job:test;",
		ls.With("container", "service").String())
	assert.Equal(t,
		"__name__:example;container:~unknown;flags:empty;image:added;instance:instance;job:test;",
		ls.With("image", "added").String())
}

func TestLabelSet_WithPairs(t *testing.T) {
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
	assert.Equal(t,
		"__name__:example;container:service;flags:empty;image:added;instance:instance;job:test;",
		merged.String())
}

func TestLabelSet_Merge(t *testing.T) {
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
	assert.Equal(t,
		"__name__:example;container:service;flags:empty;image:added;instance:instance;job:test;",
		merged.String())
}

func TestLabelSet_Merge_empty(t *testing.T) {
	ls := model.LabelSetFromPairs(
		"__name__", "example",
		"instance", "instance",
		"job", "test",
		"container", "~unknown",
		"flags", "empty",
	)
	merged := ls.Merge(model.EmptyLabelSet())
	assert.Equal(t,
		"__name__:example;container:~unknown;flags:empty;instance:instance;job:test;",
		merged.String())
}

func TestLabelSet_Split(t *testing.T) {
	ls := model.LabelSetFromPairs(
		"__name__", "example",
		"instance", "instance",
		"job", "test",
		"container", "~unknown",
		"flags", "empty",
	)
	extracted, rest := ls.SplitBy("flags", "container", "image")
	assert.Equal(t, "container:~unknown;flags:empty;", extracted.String())
	assert.Equal(t, "__name__:example;instance:instance;job:test;", rest.String())
}

func TestLabelSet_Map_Quick(t *testing.T) {
	identity := func(m map[string]string) map[string]string { return m }
	convertation := func(m map[string]string) map[string]string {
		return model.LabelSetFromMap(m).ToMap()
	}
	require.NoError(t, quick.CheckEqual(identity, convertation, nil))
}

func TestLabelSet_MarshalJSON_Quick(t *testing.T) {
	identity := func(m map[string]string) map[string]string { return m }
	convertation := func(m map[string]string) map[string]string {
		ls := model.LabelSetFromMap(m)
		blob, err := json.Marshal(ls)
		require.NoError(t, err)
		res := map[string]string{}
		err = json.Unmarshal(blob, &res)
		require.NoError(t, err)
		return res
	}
	require.NoError(t, quick.CheckEqual(identity, convertation, nil))
}

func TestLabelSet_UnmarshalJSON_Quick(t *testing.T) {
	identity := func(m map[string]string) map[string]string { return m }
	convertation := func(m map[string]string) map[string]string {
		blob, err := json.Marshal(m)
		require.NoError(t, err)
		var ls model.LabelSet
		err = json.Unmarshal(blob, &ls)
		require.NoError(t, err)
		return ls.ToMap()
	}
	require.NoError(t, quick.CheckEqual(identity, convertation, nil))
}

func TestLabelSet_MarshalYAML_Quick(t *testing.T) {
	identity := func(m map[string]string) map[string]string { return m }
	convertation := func(m map[string]string) map[string]string {
		ls := model.LabelSetFromMap(m)
		blob, err := yaml.Marshal(ls)
		require.NoError(t, err)
		res := map[string]string{}
		err = yaml.Unmarshal(blob, &res)
		require.NoError(t, err)
		return res
	}
	require.NoError(t, quick.CheckEqual(identity, convertation, nil))
}

func TestLabelSet_UnmarshalYAML_Quick(t *testing.T) {
	identity := func(m map[string]string) map[string]string { return m }
	convertation := func(m map[string]string) map[string]string {
		blob, err := yaml.Marshal(m)
		require.NoError(t, err)
		var ls model.LabelSet
		err = yaml.Unmarshal(blob, &ls)
		require.NoError(t, err)
		return ls.ToMap()
	}
	require.NoError(t, quick.CheckEqual(identity, convertation, nil))
}
