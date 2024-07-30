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

  IndexWriter(const QueryableEncodingBimap& lss, const ChunkMetadataList& chunk_metadata_list) : lss_(lss), chunk_metadata_list_(chunk_metadata_list) {}

  PROMPP_ALWAYS_INLINE void write_header(std::ostream& stream) {
    StreamWriter writer(stream);

    writer.write_uint32(PromPP::Prometheus::tsdb::index::kMagic);
    writer.write(PromPP::Prometheus::tsdb::index::kFormatVersion);

    position_ += writer.position();
  }

  PROMPP_ALWAYS_INLINE void write_symbols(std::ostream& stream) {
    StreamWriter writer(stream);
    section_writer::SymbolsWriter{lss_, symbol_references_, writer}.write();

    toc_.symbols = position_;
    position_ += writer.position();
  }

  void write(std::ostream& stream) {
    write_header(stream);

    StreamWriter writer(stream);

    toc_.symbols = writer.position();
    section_writer::SymbolsWriter{lss_, symbol_references_, writer}.write();

    toc_.series = writer.position();
    section_writer::SeriesWriter{lss_, chunk_metadata_list_, symbol_references_, series_references_, writer}.write();

    toc_.label_indices = writer.position();
    section_writer::LabelIndicesWriter label_indices_writer(lss_, symbol_references_, writer);
    label_indices_writer.write_label_indices();

    toc_.postings = writer.position();
    section_writer::PostingsWriter postings_writer(lss_, series_references_, writer);
    postings_writer.write_postings();

    toc_.label_indices_table = writer.position();
    label_indices_writer.write_label_indices_table();

    toc_.postings_offset_table = writer.position();
    postings_writer.write_postings_table_offsets();

    PromPP::Prometheus::tsdb::index::TocWriter{toc_, writer}.write();
  }

 private:
  const QueryableEncodingBimap& lss_;
  const ChunkMetadataList& chunk_metadata_list_;

  SymbolReferencesMap symbol_references_;
  SeriesReferencesMap series_references_;

  PromPP::Prometheus::tsdb::index::Toc toc_;

  uint32_t position_{};
};

}  // namespace series_index::prometheus::tsdb::index