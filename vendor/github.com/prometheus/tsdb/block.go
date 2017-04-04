package tsdb

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"github.com/oklog/ulid"
	"github.com/pkg/errors"
)

// DiskBlock handles reads against a Block of time series data.
type DiskBlock interface {
	// Directory where block data is stored.
	Dir() string

	// Stats returns statistics about the block.
	Meta() BlockMeta

	// Index returns an IndexReader over the block's data.
	Index() IndexReader

	// Series returns a SeriesReader over the block's data.
	Chunks() ChunkReader

	// Close releases all underlying resources of the block.
	Close() error
}

// Block is an interface to a DiskBlock that can also be queried.
type Block interface {
	DiskBlock
	Queryable
}

// HeadBlock is a regular block that can still be appended to.
type HeadBlock interface {
	Block
	Appendable
}

// Appendable defines an entity to which data can be appended.
type Appendable interface {
	// Appender returns a new Appender against an underlying store.
	Appender() Appender

	// Busy returns whether there are any currently active appenders.
	Busy() bool
}

// Queryable defines an entity which provides a Querier.
type Queryable interface {
	Querier(mint, maxt int64) Querier
}

// BlockMeta provides meta information about a block.
type BlockMeta struct {
	// Unique identifier for the block and its contents. Changes on compaction.
	ULID ulid.ULID `json:"ulid"`

	// Sequence number of the block.
	Sequence int `json:"sequence"`

	// MinTime and MaxTime specify the time range all samples
	// in the block are in.
	MinTime int64 `json:"minTime"`
	MaxTime int64 `json:"maxTime"`

	// Stats about the contents of the block.
	Stats struct {
		NumSamples uint64 `json:"numSamples,omitempty"`
		NumSeries  uint64 `json:"numSeries,omitempty"`
		NumChunks  uint64 `json:"numChunks,omitempty"`
	} `json:"stats,omitempty"`

	// Information on compactions the block was created from.
	Compaction struct {
		Generation int `json:"generation"`
	} `json:"compaction"`
}

const (
	flagNone = 0
	flagStd  = 1
)

type blockMeta struct {
	Version int `json:"version"`

	*BlockMeta
}

const metaFilename = "meta.json"

func readMetaFile(dir string) (*BlockMeta, error) {
	b, err := ioutil.ReadFile(filepath.Join(dir, metaFilename))
	if err != nil {
		return nil, err
	}
	var m blockMeta

	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m.Version != 1 {
		return nil, errors.Errorf("unexpected meta file version %d", m.Version)
	}

	return m.BlockMeta, nil
}

func writeMetaFile(dir string, meta *BlockMeta) error {
	// Make any changes to the file appear atomic.
	path := filepath.Join(dir, metaFilename)
	tmp := path + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "\t")

	if err := enc.Encode(&blockMeta{Version: 1, BlockMeta: meta}); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return renameFile(tmp, path)
}

type persistedBlock struct {
	dir  string
	meta BlockMeta

	chunkr *chunkReader
	indexr *indexReader
}

func newPersistedBlock(dir string) (*persistedBlock, error) {
	meta, err := readMetaFile(dir)
	if err != nil {
		return nil, err
	}

	cr, err := newChunkReader(chunkDir(dir))
	if err != nil {
		return nil, err
	}
	ir, err := newIndexReader(dir)
	if err != nil {
		return nil, err
	}

	pb := &persistedBlock{
		dir:    dir,
		meta:   *meta,
		chunkr: cr,
		indexr: ir,
	}
	return pb, nil
}

func (pb *persistedBlock) Close() error {
	var merr MultiError

	merr.Add(pb.chunkr.Close())
	merr.Add(pb.indexr.Close())

	return merr.Err()
}

func (pb *persistedBlock) String() string {
	return fmt.Sprintf("(%d, %s)", pb.meta.Sequence, pb.meta.ULID)
}

func (pb *persistedBlock) Querier(mint, maxt int64) Querier {
	return &blockQuerier{
		mint:   mint,
		maxt:   maxt,
		index:  pb.Index(),
		chunks: pb.Chunks(),
	}
}

func (pb *persistedBlock) Dir() string         { return pb.dir }
func (pb *persistedBlock) Index() IndexReader  { return pb.indexr }
func (pb *persistedBlock) Chunks() ChunkReader { return pb.chunkr }
func (pb *persistedBlock) Meta() BlockMeta     { return pb.meta }

func chunkDir(dir string) string { return filepath.Join(dir, "chunks") }
func walDir(dir string) string   { return filepath.Join(dir, "wal") }

type mmapFile struct {
	f *os.File
	b []byte
}

func openMmapFile(path string) (*mmapFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "try lock file")
	}
	info, err := f.Stat()
	if err != nil {
		return nil, errors.Wrap(err, "stat")
	}

	b, err := mmap(f, int(info.Size()))
	if err != nil {
		return nil, errors.Wrap(err, "mmap")
	}

	return &mmapFile{f: f, b: b}, nil
}

func (f *mmapFile) Close() error {
	err0 := munmap(f.b)
	err1 := f.f.Close()

	if err0 != nil {
		return err0
	}
	return err1
}

// A skiplist maps offsets to values. The values found in the data at an
// offset are strictly greater than the indexed value.
type skiplist interface {
	// offset returns the offset to data containing values of x and lower.
	offset(x int64) (uint32, bool)
}

// simpleSkiplist is a slice of plain value/offset pairs.
type simpleSkiplist []skiplistPair

type skiplistPair struct {
	value  int64
	offset uint32
}

func (sl simpleSkiplist) offset(x int64) (uint32, bool) {
	// Search for the first offset that contains data greater than x.
	i := sort.Search(len(sl), func(i int) bool { return sl[i].value >= x })

	// If no element was found return false. If the first element is found,
	// there's no previous offset actually containing values that are x or lower.
	if i == len(sl) || i == 0 {
		return 0, false
	}
	return sl[i-1].offset, true
}
