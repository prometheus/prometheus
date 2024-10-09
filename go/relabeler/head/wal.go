package head

import (
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"io"
)

type ShardWal struct {
	encoder     *cppbridge.HeadWalEncoder
	writeCloser io.WriteCloser
}

func NewShardWal(lss *cppbridge.LabelSetStorage, writeCloser io.WriteCloser) *ShardWal {
	return &ShardWal{
		encoder:     cppbridge.NewHeadWalEncoder(lss),
		writeCloser: writeCloser,
	}
}

func (w *ShardWal) Write(innerSeriesSlice []*cppbridge.InnerSeries) error {
	data, err := w.encoder.Encode(innerSeriesSlice)
	if err != nil {
		return err
	}

	_, err = w.writeCloser.Write(data)
	return err
}

func (w *ShardWal) Close() error {
	return w.writeCloser.Close()
}
