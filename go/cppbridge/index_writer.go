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
	writer              uintptr
	chunk_metadata_list *[][]ChunkMetadata
	lss                 *LabelSetStorage
	data                []byte
}

func NewIndexWriter(lss *LabelSetStorage, chunk_metadata_list *[][]ChunkMetadata) *IndexWriter {
	writer := &IndexWriter{
		writer:              indexWriterCtor(lss.Pointer(), chunk_metadata_list),
		chunk_metadata_list: chunk_metadata_list,
		lss:                 lss,
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

func (writer *IndexWriter) WriteNextSeriesBatch(batch_size uint32) ([]byte, bool) {
	var has_more_data bool
	writer.data, has_more_data = indexWriterWriteNextSeriesBatch(writer.writer, batch_size, writer.data)
	return writer.data, has_more_data
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
