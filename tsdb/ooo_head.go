package tsdb

import (
	"fmt"

	"github.com/prometheus/prometheus/tsdb/tombstones"
)

var _ BlockReader = &OOORangeHead{}

// OOORangeHead allows querying Head out of order samples via BlockReader
// interface implementation.
type OOORangeHead struct {
	head *Head
	// mint and maxt are tracked because when a query is handled we only want
	// the timerange of the query and having preexisting pointers to the first
	// and last timestamp help with that.
	mint, maxt int64
}

func NewOOORangeHead(head *Head, mint, maxt int64) *OOORangeHead {
	return &OOORangeHead{
		head: head,
		mint: mint,
		maxt: maxt,
	}
}

func (oh *OOORangeHead) Index() (IndexReader, error) {
	return NewOOOHeadIndexReader(oh.head, oh.mint, oh.maxt), nil
}

func (oh *OOORangeHead) Chunks() (ChunkReader, error) {
	return NewOOOHeadChunkReader(oh.head, oh.mint, oh.maxt), nil
}

func (oh *OOORangeHead) Tombstones() (tombstones.Reader, error) {
	// As stated in the design doc https://docs.google.com/document/d/1Kppm7qL9C-BJB1j6yb6-9ObG3AbdZnFUBYPNNWwDBYM/edit?usp=sharing
	// Tombstones are not supported for out of order metrics.
	return tombstones.NewMemTombstones(), nil
}

func (oh *OOORangeHead) Meta() BlockMeta {
	var id [16]byte
	copy(id[:], "____ooo_head____")
	return BlockMeta{
		MinTime: oh.mint,
		MaxTime: oh.maxt,
		ULID:    id,
		Stats: BlockStats{
			NumSeries: oh.head.NumSeries(),
		},
	}
}

// Size returns the size taken by the Head block.
func (oh *OOORangeHead) Size() int64 {
	return oh.head.Size()
}

// String returns an human readable representation of the out of order range
// head. It's important to keep this function in order to avoid the struct dump
// when the head is stringified in errors or logs.
func (oh *OOORangeHead) String() string {
	return fmt.Sprintf("ooo range head (mint: %d, maxt: %d)", oh.MinTime(), oh.MaxTime())
}

func (oh *OOORangeHead) MinTime() int64 {
	return oh.mint
}

func (oh *OOORangeHead) MaxTime() int64 {
	return oh.maxt
}
