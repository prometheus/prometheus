package tsdb

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/pkg/errors"
)

// Block handles reads against a block of time series data.
type block interface {
	dir() string
	stats() BlockStats
	interval() (int64, int64)
	index() IndexReader
	series() SeriesReader
	persisted() bool
}

type BlockStats struct {
	MinTime     int64
	MaxTime     int64
	SampleCount uint64
	SeriesCount uint32
	ChunkCount  uint32
}

const (
	flagNone = 0
	flagStd  = 1
)

type persistedBlock struct {
	d      string
	bstats BlockStats

	chunksf, indexf *mmapFile

	chunkr *seriesReader
	indexr *indexReader
}

func newPersistedBlock(p string) (*persistedBlock, error) {
	// TODO(fabxc): validate match of name and stats time, validate magic.

	// mmap files belonging to the block.
	chunksf, err := openMmapFile(chunksFileName(p))
	if err != nil {
		return nil, errors.Wrap(err, "open chunk file")
	}
	indexf, err := openMmapFile(indexFileName(p))
	if err != nil {
		return nil, errors.Wrap(err, "open index file")
	}

	sr, err := newSeriesReader(chunksf.b)
	if err != nil {
		return nil, errors.Wrap(err, "create series reader")
	}
	ir, err := newIndexReader(sr, indexf.b)
	if err != nil {
		return nil, errors.Wrap(err, "create index reader")
	}

	stats, err := ir.Stats()
	if err != nil {
		return nil, errors.Wrap(err, "read stats")
	}

	pb := &persistedBlock{
		d:       p,
		chunksf: chunksf,
		indexf:  indexf,
		chunkr:  sr,
		indexr:  ir,
		bstats:  stats,
	}
	return pb, nil
}

func (pb *persistedBlock) Close() error {
	err0 := pb.chunksf.Close()
	err1 := pb.indexf.Close()

	if err0 != nil {
		return err0
	}
	return err1
}

func (pb *persistedBlock) dir() string          { return pb.d }
func (pb *persistedBlock) persisted() bool      { return true }
func (pb *persistedBlock) index() IndexReader   { return pb.indexr }
func (pb *persistedBlock) series() SeriesReader { return pb.chunkr }
func (pb *persistedBlock) stats() BlockStats    { return pb.bstats }

func (pb *persistedBlock) interval() (int64, int64) {
	return pb.bstats.MinTime, pb.bstats.MaxTime
}

func chunksFileName(path string) string {
	return filepath.Join(path, "000-series")
}

func indexFileName(path string) string {
	return filepath.Join(path, "000-index")
}

type mmapFile struct {
	f *fileutil.LockedFile
	b []byte
}

func openMmapFile(path string) (*mmapFile, error) {
	f, err := fileutil.TryLockFile(path, os.O_RDONLY, 0666)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	b, err := mmap(f.File, int(info.Size()))
	if err != nil {
		return nil, err
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
