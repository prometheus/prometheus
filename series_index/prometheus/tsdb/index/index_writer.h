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

class IndexWriter {
 public:
  using TrieIndex = series_index::TrieIndex<series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;
  using QueryableEncodingBimap = series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, TrieIndex>;

  IndexWriter(const QueryableEncodingBimap& lss, std::ostream& stream) : lss_(lss), writer_(stream) {}

  void write() {
    write_header();

    toc_.symbols = writer_.position();
    section_writer::SymbolsWriter{lss_, symbol_references_, writer_}.write();

    toc_.series = writer_.position();
    section_writer::SeriesWriter{lss_, symbol_references_, series_references_, writer_}.write();

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
  PromPP::Prometheus::tsdb::index::StreamWriter writer_;

  SymbolReferencesMap symbol_references_;
  SeriesReferencesMap series_references_;

  PromPP::Prometheus::tsdb::index::Toc toc_;

  PROMPP_ALWAYS_INLINE void write_header() {
    writer_.write_uint32(PromPP::Prometheus::tsdb::index::kMagic);
    writer_.write(PromPP::Prometheus::tsdb::index::kFormatVersion);
  }
};

}  // namespace series_index::prometheus::tsdb::index