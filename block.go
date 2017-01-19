package tsdb

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/pkg/errors"
)

// Block handles reads against a Block of time series data.
type Block interface {
	// Directory where block data is stored.
	Dir() string

	// Stats returns statistics about the block.
	Meta() BlockMeta

	// Index returns an IndexReader over the block's data.
	Index() IndexReader

	// Series returns a SeriesReader over the block's data.
	Series() SeriesReader

	// Persisted returns whether the block is already persisted,
	// and no longer being appended to.
	Persisted() bool

	// Close releases all underlying resources of the block.
	Close() error
}

// BlockMeta provides meta information about a block.
type BlockMeta struct {
	MinTime int64 `json:"minTime,omitempty"`
	MaxTime int64 `json:"maxTime,omitempty"`

	Stats struct {
		NumSamples uint64 `json:"numSamples,omitempty"`
		NumSeries  uint64 `json:"numSeries,omitempty"`
		NumChunks  uint64 `json:"numChunks,omitempty"`
	} `json:"stats,omitempty"`
}

const (
	flagNone = 0
	flagStd  = 1
)

type persistedBlock struct {
	dir  string
	meta BlockMeta

	chunksf, indexf *mmapFile

	chunkr *seriesReader
	indexr *indexReader
}

type blockMeta struct {
	Version int       `json:"version"`
	Meta    BlockMeta `json:",inline"`
}

const metaFilename = "meta.json"

func newPersistedBlock(dir string) (*persistedBlock, error) {
	// TODO(fabxc): validate match of name and stats time, validate magic.

	// mmap files belonging to the block.
	chunksf, err := openMmapFile(chunksFileName(dir))
	if err != nil {
		return nil, errors.Wrap(err, "open chunk file")
	}
	indexf, err := openMmapFile(indexFileName(dir))
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

	pb := &persistedBlock{
		dir:     dir,
		chunksf: chunksf,
		indexf:  indexf,
		chunkr:  sr,
		indexr:  ir,
	}

	b, err := ioutil.ReadFile(filepath.Join(dir, metaFilename))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &pb.meta); err != nil {
		return nil, err
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

func (pb *persistedBlock) Dir() string          { return pb.dir }
func (pb *persistedBlock) Persisted() bool      { return true }
func (pb *persistedBlock) Index() IndexReader   { return pb.indexr }
func (pb *persistedBlock) Series() SeriesReader { return pb.chunkr }
func (pb *persistedBlock) Meta() BlockMeta      { return pb.meta }

func chunksFileName(path string) string {
	return filepath.Join(path, "chunks-000")
}

func indexFileName(path string) string {
	return filepath.Join(path, "index-000")
}

type mmapFile struct {
	f *fileutil.LockedFile
	b []byte
}

func openMmapFile(path string) (*mmapFile, error) {
	// We have to open the file in RDWR for the lock to work with fileutil.
	// TODO(fabxc): use own flock call that supports multi-reader.
	f, err := fileutil.TryLockFile(path, os.O_RDWR, 0666)
	if err != nil {
		return nil, errors.Wrap(err, "try lock file")
	}
	info, err := f.Stat()
	if err != nil {
		return nil, errors.Wrap(err, "stat")
	}

	b, err := mmap(f.File, int(info.Size()))
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
