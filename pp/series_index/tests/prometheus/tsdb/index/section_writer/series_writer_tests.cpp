#include <gtest/gtest.h>

#include "primitives/label_set.h"
#include "primitives/snug_composites.h"
#include "series_index/prometheus/tsdb/index/section_writer/series_writer.h"
#include "series_index/prometheus/tsdb/index/section_writer/symbols_writer.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using series_index::prometheus::tsdb::index::ChunkMetadata;
using series_index::prometheus::tsdb::index::SeriesReferencesMap;
using series_index::prometheus::tsdb::index::SymbolReferencesMap;
using series_index::prometheus::tsdb::index::section_writer::SymbolsWriter;
using std::operator""sv;

using ChunkMetadataList = std::vector<ChunkMetadata>;
using LabelViewSetList = std::vector<LabelViewSet>;

class SeriesWriterFixture : public testing::Test {
 protected:
  using TrieIndex = series_index::TrieIndex<series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;
  using QueryableEncodingBimap =
      series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, BareBones::Vector, TrieIndex>;
  using Stream = std::ostringstream;
  using StreamWriter = PromPP::Prometheus::tsdb::index::StreamWriter<Stream>;
  using SeriesWriter = series_index::prometheus::tsdb::index::section_writer::SeriesWriter<QueryableEncodingBimap, Stream>;

  Stream stream_;
  StreamWriter stream_writer_{&stream_};
  QueryableEncodingBimap lss_;
  SymbolReferencesMap symbol_references_;
  SeriesReferencesMap series_references_;

  void fill_lss_and_symbols(const LabelViewSetList& label_sets) {
    for (auto& label_set : label_sets) {
      lss_.find_or_emplace(label_set);
    }

    std::ostringstream stream;
    StreamWriter stream_writer{&stream};
    SymbolsWriter{lss_, symbol_references_, stream_writer}.write();
  }
};

TEST_F(SeriesWriterFixture, OneChunk) {
  // Arrange
  fill_lss_and_symbols({{{"job", "cron"}, {"server", "localhost"}}});
  SeriesWriter series_writer{lss_, symbol_references_, series_references_};

  // Act
  series_writer.write(0, ChunkMetadataList{{.min_timestamp = 1000, .max_timestamp = 2001, .reference = 0}}, stream_writer_);

  // Assert
  EXPECT_EQ(
      "\x0B"
      "\x02"
      "\x02\x01"
      "\x04\x03"

      "\x01"

      "\xD0\x0F"
      "\xE9\x07"
      "\x00"

      "\xE3\x66\x88\x29"sv,
      stream_.view());
}

TEST_F(SeriesWriterFixture, MultiplySeriesMultiplyChunks) {
  // Arrange
  fill_lss_and_symbols({
      {{"job", "cron"}, {"server", "127.0.0.1"}},
      {{"job", "cron"}, {"server", "remote"}},
      {{"job", "cron"}, {"server", "localhost"}},
  });
  SeriesWriter series_writer{lss_, symbol_references_, series_references_};
  const ChunkMetadataList chunks = {{
      {.min_timestamp = 1000, .max_timestamp = 2001, .reference = 0},
      {.min_timestamp = 2002, .max_timestamp = 4004, .reference = 100},
      {.min_timestamp = 4005, .max_timestamp = 5005, .reference = 125},
  }};

  // Act
  series_writer.write(0, chunks, stream_writer_);
  series_writer.write(2, chunks, stream_writer_);

  // Assert
  EXPECT_EQ(
      "\x14"
      "\x02"
      "\x03\x02"
      "\x06\x01"

      "\x03"

      "\xD0\x0F"
      "\xE9\x07"
      "\x00"

      "\x01"
      "\xD2\x0F"
      "\xC8\x01"

      "\x01"
      "\xE8\x07"
      "\x32"

      "\xF5\x2E\x73\xFB"
      "\x00\x00\x00\x00\x00\x00\x00"

      "\x14"
      "\x02"
      "\x03\x02"
      "\x06\x04"

      "\x03"

      "\xD0\x0F"
      "\xE9\x07"
      "\x00"

      "\x01"
      "\xD2\x0F"
      "\xC8\x01"

      "\x01"
      "\xE8\x07"
      "\x32"

      "\xC1\x26\xD2\xEE"
      "\x00\x00\x00\x00\x00\x00\x00"sv,
      stream_.view());
}

}  // namespace
