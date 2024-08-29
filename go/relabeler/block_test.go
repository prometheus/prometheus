package relabeler

import (
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler/block"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewBlock(t *testing.T) {
	lss := cppbridge.NewQueryableLssStorage()
	ds := cppbridge.NewHeadDataStorage()
	ls := model.NewLabelSetBuilder().Set("__name__", "this_is_obviuosly_the_best_metric").Set("lol", "kek").Build()
	lsID := lss.FindOrEmplace(ls)

	enc := cppbridge.NewHeadEncoderWithDataStorage(ds)
	ts := time.Now()
	enc.Encode(lsID, ts.UnixMilli(), 0)
	enc.Encode(lsID, ts.Add(time.Minute).UnixMilli(), 1)
	enc.Encode(lsID, ts.Add(time.Minute*2).UnixMilli(), 2)

	dir, err := os.Getwd()
	require.NoError(t, err)
	t.Log(dir)

	dir = filepath.Join(dir, "data")
	blockWriter := block.NewBlockWriter(dir, block.DefaultChunkSegmentSize)
	err = blockWriter.Write(NewBlock(lss, ds))
	require.NoError(t, err)

}
