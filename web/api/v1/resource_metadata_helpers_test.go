// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0.

package v1

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

func TestDiffStringMap(t *testing.T) {
	before := map[string]string{"service.version": "1.0.0", "host.name": "a"}
	after := map[string]string{"service.version": "1.0.1", "host.name": "a", "cloud.region": "us"}
	got := diffStringMap(before, after)
	require.Equal(t, "1.0.0->1.0.1", got["service.version"])
	require.Equal(t, "us", got["cloud.region"])
	_, ok := got["host.name"]
	require.False(t, ok)
}

func TestApplyLatestVersions(t *testing.T) {
	mk := func() []ResourceAttributesResponse {
		return []ResourceAttributesResponse{{
			Labels: labels.FromStrings("__name__", "m"),
			Versions: []ResourceAttributeVersion{
				{MinTimeMs: 1, MaxTimeMs: 10, Attributes: ResourceAttributeData{Descriptive: map[string]string{"service.version": "1"}}},
				{MinTimeMs: 11, MaxTimeMs: 20, Attributes: ResourceAttributeData{Descriptive: map[string]string{"service.version": "2"}}},
			},
		}}
	}
	out := applyLatestVersions(mk(), true)
	require.Len(t, out[0].Versions, 1)
	require.Equal(t, "2", out[0].Versions[0].Attributes.Descriptive["service.version"])
	require.Equal(t, int64(20), out[0].Versions[0].MaxTimeMs)

	out2 := applyLatestVersions(mk(), false)
	require.Len(t, out2[0].Versions, 2)
}
