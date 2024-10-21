#include <gtest/gtest.h>

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

using ChunkMetadataList = std::vector<std::vector<ChunkMetadata>>;
using LabelViewSetList = std::vector<LabelViewSet>;

struct SeriesWriterCase {
  LabelViewSetList label_sets;
  ChunkMetadataList chunk_metadata_list;
  uint32_t series_count;
  std::string_view expected;
};

class SeriesWriterFixture : public testing::TestWithParam<SeriesWriterCase> {
 protected:
  using TrieIndex = series_index::TrieIndex<series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;
  using QueryableEncodingBimap = series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, TrieIndex>;
  using Stream = std::ostringstream;
  using StreamWriter = PromPP::Prometheus::tsdb::index::StreamWriter<Stream>;
  using SeriesWriter = series_index::prometheus::tsdb::index::section_writer::SeriesWriter<QueryableEncodingBimap, ChunkMetadataList, Stream>;

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

TEST_P(SeriesWriterFixture, FullWrite) {
  // Arrange
  fill_lss_and_symbols(GetParam().label_sets);
  SeriesWriter series_writer{lss_, GetParam().chunk_metadata_list, symbol_references_, series_references_};

  // Act
  series_writer.write(stream_writer_, GetParam().series_count);

  // Assert
  EXPECT_EQ(GetParam().expected, stream_.view());
  EXPECT_FALSE(series_writer.has_more_data());
}

INSTANTIATE_TEST_SUITE_P(EmptyLabelSet,
                         SeriesWriterFixture,
                         testing::Values(SeriesWriterCase{.label_sets = {}, .chunk_metadata_list = {}, .series_count = 1, .expected = ""}));

INSTANTIATE_TEST_SUITE_P(SeriesWithEmptyChunks,
                         SeriesWriterFixture,
                         testing::Values(SeriesWriterCase{.label_sets = {{{"job", "cron"}}}, .chunk_metadata_list = {{}}, .series_count = 1, .expected = ""}));

INSTANTIATE_TEST_SUITE_P(TwoSeries,
                         SeriesWriterFixture,
                         testing::Values(SeriesWriterCase{.label_sets =
                                                              {
                                                                  {{"job", "cron"}, {"server", "localhost"}},
                                                                  {{"job", "cron"}, {"server", "remote"}},
                                                                  {{"job", "cron"}, {"server", "127.0.0.1"}},
                                                              },
                                                          .chunk_metadata_list =
                                                              {
                                                                  {{.min_timestamp = 1000, .max_timestamp = 2001, .reference = 0},
                                                                   {.min_timestamp = 2002, .max_timestamp = 4004, .reference = 100},
                                                                   {.min_timestamp = 4005, .max_timestamp = 5005, .reference = 125}},
                                                                  {},
                                                                  {{.min_timestamp = 1000, .max_timestamp = 2001, .reference = 0},
                                                                   {.min_timestamp = 2002, .max_timestamp = 4004, .reference = 100},
                                                                   {.min_timestamp = 4005, .max_timestamp = 5005, .reference = 125}},
                                                              },
                                                          .series_count = 2,
                                                          .expected = "\x14"
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
                                                                      "\x00\x00\x00\x00\x00\x00\x00"sv}));

TEST_F(SeriesWriterFixture, PartialWrite) {
  // Arrange
  fill_lss_and_symbols(LabelViewSetList{
      {{"job", "cron"}, {"server", "localhost"}},
      {{"job", "cron"}, {"server", "remote"}},
      {{"job", "cron"}, {"server", "127.0.0.1"}},
  });
  const ChunkMetadataList chunk_metadata_list{
      {{.min_timestamp = 1000, .max_timestamp = 2001, .reference = 0},
       {.min_timestamp = 2002, .max_timestamp = 4004, .reference = 100},
       {.min_timestamp = 4005, .max_timestamp = 5005, .reference = 125}},
      {},
      {{.min_timestamp = 1000, .max_timestamp = 2001, .reference = 0},
       {.min_timestamp = 2002, .max_timestamp = 4004, .reference = 100},
       {.min_timestamp = 4005, .max_timestamp = 5005, .reference = 125}},
  };
  SeriesWriter series_writer{lss_, chunk_metadata_list, symbol_references_, series_references_};

  // Act
  series_writer.write(stream_writer_, 1);
  const auto has_more_data_after_first_write = series_writer.has_more_data();
  const auto first_series_data = stream_.str();
  stream_.str("");
  series_writer.write(stream_writer_, 1);

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
      "\x00\x00\x00\x00\x00\x00\x00"sv,
      first_series_data);
  EXPECT_EQ(
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
  EXPECT_TRUE(has_more_data_after_first_write);
  EXPECT_FALSE(series_writer.has_more_data());
}

}  // namespace
