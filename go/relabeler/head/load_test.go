package head_test

import (
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/head"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "head_wal_test")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cfgs := []*config.InputRelabelerConfig{
		{},
	}
	h, err := head.Load(0, tmpDir, cfgs, 2, prometheus.DefaultRegisterer)
	require.NoError(t, err)
	require.NoError(t, h.Close())
}
