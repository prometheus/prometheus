package tsdb

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// Block handles reads against a block of time series data within a time window.
type Block interface {
	Querier(mint, maxt int64) Querier
}

const (
	flagNone = 0
	flagStd  = 1
)

type persistedBlock struct {
	chunksf, indexf *mmapFile

	chunks *seriesReader
	index  *indexReader

	baseTime int64
}

func newPersistedBlock(path string) (*persistedBlock, error) {
	// The directory must be named after the base timestamp for the block.
	baset, err := strconv.ParseInt(filepath.Base(path), 10, 0)
	if err != nil {
		return nil, fmt.Errorf("unexpected directory name %q: %s", path, err)
	}

	// mmap files belonging to the block.
	chunksf, err := openMmapFile(chunksFileName(path))
	if err != nil {
		return nil, err
	}
	indexf, err := openMmapFile(indexFileName(path))
	if err != nil {
		return nil, err
	}

	sr, err := newSeriesReader(chunksf.b)
	if err != nil {
		return nil, err
	}
	ir, err := newIndexReader(sr, indexf.b)
	if err != nil {
		return nil, err
	}

	pb := &persistedBlock{
		chunksf:  chunksf,
		indexf:   indexf,
		chunks:   sr,
		index:    ir,
		baseTime: baset,
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

type persistedBlocks []*persistedBlock

func (p persistedBlocks) Len() int           { return len(p) }
func (p persistedBlocks) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p persistedBlocks) Less(i, j int) bool { return p[i].baseTime < p[j].baseTime }

// findBlocks finds time-ordered persisted blocks within a directory.
func findPersistedBlocks(path string) ([]*persistedBlock, error) {
	var pbs persistedBlocks

	files, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}

	for _, fi := range files {
		pb, err := newPersistedBlock(fi.Name())
		if err != nil {
			return nil, fmt.Errorf("error initializing block %q: %s", fi.Name(), err)
		}
		pbs = append(pbs, pb)
	}

	// Order blocks by their base time so they represent a continous
	// range of time.
	sort.Sort(pbs)

	return pbs, nil
}

func chunksFileName(path string) string {
	return filepath.Join(path, "series")
}

func indexFileName(path string) string {
	return filepath.Join(path, "index")
}

type mmapFile struct {
	f *os.File
	b []byte
}

func openMmapFile(path string) (*mmapFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	b, err := mmap(f, int(info.Size()))
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
