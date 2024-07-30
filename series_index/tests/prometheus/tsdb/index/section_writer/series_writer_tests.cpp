#include <gtest/gtest.h>

#include "primitives/snug_composites.h"
#include "series_index/prometheus/tsdb/index/section_writer/series_writer.h"
#include "series_index/prometheus/tsdb/index/section_writer/symbols_writer.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Prometheus::tsdb::index::StreamWriter;
using series_index::prometheus::tsdb::index::ChunkMetadata;
using series_index::prometheus::tsdb::index::SeriesReferencesMap;
using series_index::prometheus::tsdb::index::SymbolReferencesMap;
using series_index::prometheus::tsdb::index::section_writer::SeriesWriter;
using series_index::prometheus::tsdb::index::section_writer::SymbolsWriter;
using std::operator""sv;

using ChunkMetadataList = std::vector<std::vector<ChunkMetadata>>;

struct SeriesWriterCase {
  std::vector<LabelViewSet> label_sets;
  ChunkMetadataList chunk_metadata_list;
  std::string_view expected;
};

class SeriesWriterFixture : public testing::TestWithParam<SeriesWriterCase> {
 protected:
  using TrieIndex = series_index::TrieIndex<series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;
  using QueryableEncodingBimap = series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, TrieIndex>;
  using ChunkMetadataList = std::vector<std::vector<ChunkMetadata>>;

  std::ostringstream stream_;
  StreamWriter stream_writer_{stream_};
  QueryableEncodingBimap lss_;
  SymbolReferencesMap symbol_references_;
  SeriesReferencesMap series_references_;
  SeriesWriter<QueryableEncodingBimap, ChunkMetadataList> series_writer_{lss_, GetParam().chunk_metadata_list, symbol_references_, series_references_,
                                                                         stream_writer_};

  void SetUp() final {
    for (auto& label_set : GetParam().label_sets) {
      lss_.find_or_emplace(label_set);
    }

    SymbolsWriter<QueryableEncodingBimap>{lss_, symbol_references_, stream_writer_}.write();

    stream_.str("");
    stream_.clear();
  }
};

TEST_P(SeriesWriterFixture, Test) {
  // Arrange

  // Act
  series_writer_.write();

  // Assert
  EXPECT_EQ(GetParam().expected, stream_.view());
}

INSTANTIATE_TEST_SUITE_P(EmptyLabelSet, SeriesWriterFixture, testing::Values(SeriesWriterCase{.label_sets = {}, .chunk_metadata_list = {}, .expected = ""}));

INSTANTIATE_TEST_SUITE_P(LabelWithEmptyValue,
                         SeriesWriterFixture,
                         testing::Values(SeriesWriterCase{.label_sets = {{{"key", ""}}},
                                                          .chunk_metadata_list = {{}},
                                                          .expected = "\x04"
                                                                      "\x01"
                                                                      "\x01"
                                                                      "\x00"
                                                                      "\x00"
                                                                      "\x30\x63\x73\x01"
                                                                      "\x00\x00\x00\x00\x00\x00\x00"sv}));

INSTANTIATE_TEST_SUITE_P(TwoSeries,
                         SeriesWriterFixture,
                         testing::Values(SeriesWriterCase{
                             .label_sets = {{{"job", "cron"}, {"server", "localhost"}}, {{"job", "cron"}, {"server", "127.0.0.1"}}},
                             .chunk_metadata_list = {{}, {}},
                             .expected = "\x06"
                                         "\x02"
                                         "\x03\x02"
                                         "\x05\x01"
                                         "\x00"
                                         "\x53\xCF\xE1\x2F"
                                         "\x00\x00\x00\x00\x00"

                                         "\x06"
                                         "\x02"
                                         "\x03\x02"
                                         "\x05\x04"
                                         "\x00"
                                         "\x0E\xE7\x18\x84"
                                         "\x00\x00\x00\x00\x00"sv}));

INSTANTIATE_TEST_SUITE_P(WithChunks,
                         SeriesWriterFixture,
                         testing::Values(SeriesWriterCase{.label_sets = {{{"job", "cron"}}},
                                                          .chunk_metadata_list = {{{.min_timestamp = 1000, .max_timestamp = 2001, .size = 100},
                                                                                   {.min_timestamp = 2002, .max_timestamp = 4004, .size = 125}}},
                                                          .expected = "\x0E"
                                                                      "\x01"
                                                                      "\x02"
                                                                      "\x01"

                                                                      "\x02"

                                                                      "\xD0\x0F"
                                                                      "\xE9\x07"
                                                                      "\x00"

                                                                      "\x01"
                                                                      "\xD2\x0F"
                                                                      "\xC8\x01"

                                                                      "\x69\xFD\xD7\x80"

                                                                      "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"sv}));

}  // namespace