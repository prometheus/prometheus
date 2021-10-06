package tsdb

import (
	"container/heap"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/golang/snappy"

	"github.com/prometheus/prometheus/tsdb/errors"
)

// symbolsBatcher keeps buffer of symbols in memory. Once the buffer reaches the size limit (number of symbols),
// batcher writes currently buffered symbols to file. At the end remaining symbols must be flushed. After writing
// all batches, symbolsBatcher has list of files that can be used together with newSymbolsIterator to iterate
// through all previously added symbols in sorted order.
type symbolsBatcher struct {
	dir   string
	limit int

	buffer       map[string]struct{} // using map to deduplicate
	symbolsFiles []string            // paths of symbol files that have been successfully written.
}

func newSymbolsBatcher(limit int, dir string) *symbolsBatcher {
	return &symbolsBatcher{
		limit:  limit,
		dir:    dir,
		buffer: make(map[string]struct{}, limit),
	}
}

func (sw *symbolsBatcher) addSymbol(sym string) error {
	sw.buffer[sym] = struct{}{}
	return sw.flushSymbols(false)
}

func (sw *symbolsBatcher) flushSymbols(force bool) error {
	if !force && len(sw.buffer) < sw.limit {
		return nil
	}

	sortedSymbols := make([]string, 0, len(sw.buffer))
	for s := range sw.buffer {
		sortedSymbols = append(sortedSymbols, s)
	}
	sort.Strings(sortedSymbols)

	symbolsFile := filepath.Join(sw.dir, fmt.Sprintf("symbols_%d", len(sw.symbolsFiles)))
	err := writeSymbolsToFile(symbolsFile, sortedSymbols)
	if err == nil {
		sw.buffer = make(map[string]struct{}, sw.limit)
		sw.symbolsFiles = append(sw.symbolsFiles, symbolsFile)
	}

	return err
}

func (sw *symbolsBatcher) symbolFiles() []string {
	return sw.symbolsFiles
}

func writeSymbolsToFile(filename string, symbols []string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	// Snappy is used for buffering and to create smaller files.
	sn := snappy.NewBufferedWriter(f)
	enc := gob.NewEncoder(sn)

	errs := errors.NewMulti()

	for _, s := range symbols {
		err := enc.Encode(s)
		if err != nil {
			errs.Add(err)
			break
		}
	}

	errs.Add(sn.Close())
	errs.Add(f.Close())
	return errs.Err()
}

// Implements heap.Interface using symbols from files.
type symbolsHeap []*symbolsFile

// Len implements sort.Interface.
func (s *symbolsHeap) Len() int {
	return len(*s)
}

// Less implements sort.Interface.
func (s *symbolsHeap) Less(i, j int) bool {
	iw, ierr := (*s)[i].Peek()
	if ierr != nil {
		// Empty string will be sorted first, so error will be returned before any other result.
		iw = ""
	}

	jw, jerr := (*s)[j].Peek()
	if jerr != nil {
		jw = ""
	}

	return iw < jw
}

// Swap implements sort.Interface.
func (s *symbolsHeap) Swap(i, j int) {
	(*s)[i], (*s)[j] = (*s)[j], (*s)[i]
}

// Push implements heap.Interface. Push should add x as element Len().
func (s *symbolsHeap) Push(x interface{}) {
	*s = append(*s, x.(*symbolsFile))
}

// Pop implements heap.Interface. Pop should remove and return element Len() - 1.
func (s *symbolsHeap) Pop() interface{} {
	l := len(*s)
	res := (*s)[l-1]
	*s = (*s)[:l-1]
	return res
}

type symbolsIterator struct {
	files []*os.File
	heap  symbolsHeap

	// To avoid returning duplicates, we remember last returned symbol. We want to support "" as a valid
	// symbol, so we use pointer to a string instead.
	lastReturned *string
}

func newSymbolsIterator(filenames []string) (*symbolsIterator, error) {
	files, err := openFiles(filenames)
	if err != nil {
		return nil, err
	}

	var symFiles []*symbolsFile
	for _, f := range files {
		symFiles = append(symFiles, newSymbolsFile(f))
	}

	h := &symbolsIterator{
		files: files,
		heap:  symFiles,
	}

	heap.Init(&h.heap)

	return h, nil
}

// NextSymbol advances iterator forward, and returns next symbol.
// If there is no next element, returns err == io.EOF.
func (sit *symbolsIterator) NextSymbol() (string, error) {
	for len(sit.heap) > 0 {
		result, err := sit.heap[0].Next()
		if err == io.EOF {
			// End of file, remove it from heap, and try next file.
			heap.Remove(&sit.heap, 0)
			continue
		}

		if err != nil {
			return "", err
		}

		heap.Fix(&sit.heap, 0)

		if sit.lastReturned != nil && *sit.lastReturned == result {
			// Duplicate symbol, try next one.
			continue
		}

		sit.lastReturned = &result
		return result, nil
	}

	return "", io.EOF
}

// Close all files.
func (sit *symbolsIterator) Close() error {
	errs := errors.NewMulti()
	for _, f := range sit.files {
		errs.Add(f.Close())
	}
	return errs.Err()
}

type symbolsFile struct {
	dec *gob.Decoder

	nextValid  bool // if true, nextSymbol and nextErr have the next symbol (possibly "")
	nextSymbol string
	nextErr    error
}

func newSymbolsFile(f *os.File) *symbolsFile {
	sn := snappy.NewReader(f)
	dec := gob.NewDecoder(sn)

	return &symbolsFile{
		dec: dec,
	}
}

// Peek returns next symbol or error, but also preserves them for subsequent Peek or Next calls.
func (sf *symbolsFile) Peek() (string, error) {
	if sf.nextValid {
		return sf.nextSymbol, sf.nextErr
	}

	sf.nextValid = true
	sf.nextSymbol, sf.nextErr = sf.readNext()
	return sf.nextSymbol, sf.nextErr
}

// Next advances iterator and returns the next symbol or error.
func (sf *symbolsFile) Next() (string, error) {
	if sf.nextValid {
		defer func() {
			sf.nextValid = false
			sf.nextSymbol = ""
			sf.nextErr = nil
		}()
		return sf.nextSymbol, sf.nextErr
	}

	return sf.readNext()
}

func (sf *symbolsFile) readNext() (string, error) {
	var s string
	err := sf.dec.Decode(&s)
	// Decode returns io.EOF at the end.
	if err != nil {
		return "", err
	}

	return s, nil
}

func openFiles(filenames []string) ([]*os.File, error) {
	var result []*os.File

	for _, fn := range filenames {
		f, err := os.Open(fn)

		if err != nil {
			// Close files opened so far.
			for _, sf := range result {
				_ = sf.Close()
			}
			return nil, err
		}

		result = append(result, f)
	}
	return result, nil
}
