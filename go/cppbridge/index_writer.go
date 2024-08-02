package cppbridge

import (
	"errors"
	"runtime"
)

type ChunkMetadata struct {
	MinTimestamp int64
	MaxTimestamp int64
	Size         uint32
}

type IndexWriter struct {
	writer uintptr
	data   []byte
}

func NewIndexWriter(lss uintptr, chunk_metadata_list *[][]ChunkMetadata) (*IndexWriter, error) {
	writerPtr := indexWriterCtor(lss, chunk_metadata_list)
	if writerPtr == uintptr(0) {
		return nil, errors.New("invalid lss")
	}

	writer := &IndexWriter{
		writer: writerPtr,
	}
	runtime.SetFinalizer(writer, func(writer *IndexWriter) {
		freeBytes(writer.data)
		indexWriterDtor(writer.writer)
	})
	return writer, nil
}

func (writer *IndexWriter) WriteHeader() []byte {
	indexWriterWriteHeader(writer.writer, &writer.data)
	return writer.data
}

func (writer *IndexWriter) WriteSymbols() []byte {
	indexWriterWriteSymbols(writer.writer, &writer.data)
	return writer.data
}

func (writer *IndexWriter) WriteNextSeriesBatch(batch_size uint32) ([]byte, bool) {
	has_more_data := false
	indexWriterWriteNextSeriesBatch(writer.writer, batch_size, &writer.data, &has_more_data)
	return writer.data, has_more_data
}

func (writer *IndexWriter) WriteLabelIndices() []byte {
	indexWriterWriteLabelIndices(writer.writer, &writer.data)
	return writer.data
}

func (writer *IndexWriter) WriteNextPostingsBatch(max_batch_size uint32) ([]byte, bool) {
	has_more_data := false
	indexWriterWriteNextPostingsBatch(writer.writer, max_batch_size, &writer.data, &has_more_data)
	return writer.data, has_more_data
}

func (writer *IndexWriter) WriteLabelIndicesTable() []byte {
	indexWriterWriteLabelIndicesTable(writer.writer, &writer.data)
	return writer.data
}

func (writer *IndexWriter) WritePostingsTableOffsets() []byte {
	indexWriterWritePostingsTableOffsets(writer.writer, &writer.data)
	return writer.data
}

func (writer *IndexWriter) WriteTableOfContents() []byte {
	indexWriterWriteTableOfContents(writer.writer, &writer.data)
	return writer.data
}
