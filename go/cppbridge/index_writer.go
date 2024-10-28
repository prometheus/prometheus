package cppbridge

import (
	"runtime"
)

type ChunkMetadata struct {
	MinTimestamp int64
	MaxTimestamp int64
	Reference    uint64
}

type IndexWriter struct {
	writer uintptr
	lss    *LabelSetStorage
	data   []byte
}

func NewIndexWriter(lss *LabelSetStorage) *IndexWriter {
	writer := &IndexWriter{
		writer: indexWriterCtor(lss.Pointer()),
		lss:    lss,
	}
	runtime.SetFinalizer(writer, func(writer *IndexWriter) {
		freeBytes(writer.data)
		indexWriterDtor(writer.writer)
	})
	return writer
}

func (writer *IndexWriter) WriteHeader() []byte {
	writer.data = indexWriterWriteHeader(writer.writer, writer.data)
	return writer.data
}

func (writer *IndexWriter) WriteSymbols() []byte {
	writer.data = indexWriterWriteSymbols(writer.writer, writer.data)
	return writer.data
}

func (writer *IndexWriter) WriteSeries(ls_id uint32, chunks_meta []ChunkMetadata) []byte {
	writer.data = indexWriterWriteNextSeriesBatch(writer.writer, ls_id, chunks_meta, writer.data)
	return writer.data
}

func (writer *IndexWriter) WriteLabelIndices() []byte {
	writer.data = indexWriterWriteLabelIndices(writer.writer, writer.data)
	return writer.data
}

func (writer *IndexWriter) WriteNextPostingsBatch(max_batch_size uint32) ([]byte, bool) {
	var has_more_data bool
	writer.data, has_more_data = indexWriterWriteNextPostingsBatch(writer.writer, max_batch_size, writer.data)
	return writer.data, has_more_data
}

func (writer *IndexWriter) WriteLabelIndicesTable() []byte {
	writer.data = indexWriterWriteLabelIndicesTable(writer.writer, writer.data)
	return writer.data
}

func (writer *IndexWriter) WritePostingsTableOffsets() []byte {
	writer.data = indexWriterWritePostingsTableOffsets(writer.writer, writer.data)
	return writer.data
}

func (writer *IndexWriter) WriteTableOfContents() []byte {
	writer.data = indexWriterWriteTableOfContents(writer.writer, writer.data)
	return writer.data
}
