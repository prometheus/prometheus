#pragma once

#include "primitives/snug_composites.h"
#include "prometheus/tsdb/index/toc_writer.h"
#include "section_writer/label_indices_writer.h"
#include "section_writer/postings_writer.h"
#include "section_writer/series_writer.h"
#include "section_writer/symbols_writer.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace series_index::prometheus::tsdb::index {

template <class ChunkMetadataList>
class IndexWriter {
 public:
  using TrieIndex = series_index::TrieIndex<series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;
  using QueryableEncodingBimap = series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, TrieIndex>;
  using StreamWriter = PromPP::Prometheus::tsdb::index::StreamWriter;
  using SeriesWriter = section_writer::SeriesWriter<QueryableEncodingBimap, ChunkMetadataList>;

  IndexWriter(const QueryableEncodingBimap& lss, const ChunkMetadataList& chunk_metadata_list)
      : lss_(lss), chunk_metadata_list_(chunk_metadata_list), series_writer_(lss_, chunk_metadata_list_, symbol_references_, series_references_) {}

  PROMPP_ALWAYS_INLINE void set_stream(std::ostream* stream) noexcept { writer_.set_stream(stream); }

  PROMPP_ALWAYS_INLINE void write_header(std::ostream* stream) {
    writer_.set_stream(stream);

    writer_.write_uint32(PromPP::Prometheus::tsdb::index::kMagic);
    writer_.write(PromPP::Prometheus::tsdb::index::kFormatVersion);
  }

  PROMPP_ALWAYS_INLINE void write_symbols(std::ostream* stream) {
    writer_.set_stream(stream);

    toc_.symbols = writer_.position();
    section_writer::SymbolsWriter{lss_, symbol_references_, writer_}.write();
  }

  PROMPP_ALWAYS_INLINE void write_series(std::ostream* stream, uint32_t series_count) {
    writer_.set_stream(stream);

    if (toc_.series == 0) {
      [[unlikely]];
      toc_.series = writer_.position();
    }
    series_writer_.write(writer_, series_count);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool has_more_series_data() const noexcept { return series_writer_.has_more_data(); }

  void write(std::ostream* stream) {
    write_header(stream);
    write_symbols(stream);
    write_series(stream, SeriesWriter::kAllSeries);

    toc_.label_indices = writer_.position();
    section_writer::LabelIndicesWriter label_indices_writer(lss_, symbol_references_, writer_);
    label_indices_writer.write_label_indices();

    toc_.postings = writer_.position();
    section_writer::PostingsWriter postings_writer(lss_, series_references_, writer_);
    postings_writer.write_postings();

    toc_.label_indices_table = writer_.position();
    label_indices_writer.write_label_indices_table();

    toc_.postings_offset_table = writer_.position();
    postings_writer.write_postings_table_offsets();

    PromPP::Prometheus::tsdb::index::TocWriter{toc_, writer_}.write();
  }

 private:
  const QueryableEncodingBimap& lss_;
  const ChunkMetadataList& chunk_metadata_list_;

  section_writer::SeriesWriter<QueryableEncodingBimap, ChunkMetadataList> series_writer_;

  SymbolReferencesMap symbol_references_;
  SeriesReferencesMap series_references_;

  PromPP::Prometheus::tsdb::index::Toc toc_;

  StreamWriter writer_;
};

}  // namespace series_index::prometheus::tsdb::index