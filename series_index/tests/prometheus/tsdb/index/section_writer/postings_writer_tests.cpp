#include <gtest/gtest.h>

#include "primitives/snug_composites.h"
#include "series_index/prometheus/tsdb/index/section_writer/postings_writer.h"
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
using series_index::prometheus::tsdb::index::section_writer::PostingsWriter;
using series_index::prometheus::tsdb::index::section_writer::SeriesWriter;
using series_index::prometheus::tsdb::index::section_writer::SymbolsWriter;
using std::operator""sv;

using ChunkMetadataList = std::vector<std::vector<ChunkMetadata>>;
using LabelViewSetList = std::vector<LabelViewSet>;

struct PostingsWriterCase {
  std::vector<LabelViewSet> label_sets;
  ChunkMetadataList chunk_metadata_list;
  std::string_view expected;
};

LabelViewSet make_ls_with_empty_label_value() {
  LabelViewSet ls{{"key", "value"}};
  for (auto& label : ls) {
    label.second = "";
    break;
  }
  return ls;
}

class PostingsWriterFixture : public testing::TestWithParam<PostingsWriterCase> {
 protected:
  using TrieIndex = series_index::TrieIndex<series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;
  using QueryableEncodingBimap = series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, TrieIndex>;

  std::ostringstream stream_;
  StreamWriter<decltype(stream_)> stream_writer_{&stream_};
  QueryableEncodingBimap lss_;
  SymbolReferencesMap symbol_references_;
  SeriesReferencesMap series_references_;

  void fill_data(const LabelViewSetList& label_sets, const ChunkMetadataList& chunk_metadata_list) {
    for (auto& label_set : label_sets) {
      lss_.find_or_emplace(label_set);
    }

    std::ostringstream stream;
    StreamWriter<decltype(stream_)> stream_writer{&stream};
    SymbolsWriter{lss_, symbol_references_, stream_writer}.write();
    SeriesWriter<QueryableEncodingBimap, ChunkMetadataList, decltype(stream_)>{lss_, chunk_metadata_list, symbol_references_, series_references_}.write(
        stream_writer);
  }
};

TEST_P(PostingsWriterFixture, FullWrite) {
  // Arrange
  fill_data(GetParam().label_sets, GetParam().chunk_metadata_list);
  PostingsWriter postings_writer{lss_, series_references_, stream_writer_};

  // Act
  postings_writer.write_postings();
  postings_writer.write_postings_table_offsets();

  // Assert
  EXPECT_EQ(GetParam().expected, stream_.view());
}

INSTANTIATE_TEST_SUITE_P(EmptyLabelSet,
                         PostingsWriterFixture,
                         testing::Values(PostingsWriterCase{.label_sets = {},
                                                            .chunk_metadata_list = {},
                                                            .expected = "\x00\x00\x00\x04"
                                                                        "\x00\x00\x00\x00"
                                                                        "\x48\x67\x4B\xC7"

                                                                        "\x00\x00\x00\x08"
                                                                        "\x00\x00\x00\x01"
                                                                        "\x02"
                                                                        "\x00"
                                                                        "\x00"
                                                                        "\x00"
                                                                        "\x0B\x5E\xFE\xA7"sv}));

INSTANTIATE_TEST_SUITE_P(LabelWithEmptyValue,
                         PostingsWriterFixture,
                         testing::Values(PostingsWriterCase{.label_sets = {make_ls_with_empty_label_value()},
                                                            .chunk_metadata_list = {{}},
                                                            .expected = "\x00\x00\x00\x08"
                                                                        "\x00\x00\x00\x01"
                                                                        "\x00\x00\x00\x02"
                                                                        "\x55\x02\xAD\xD1"

                                                                        "\x00\x00\x00\x08"
                                                                        "\x00\x00\x00\x01"
                                                                        "\x02"
                                                                        "\x00"
                                                                        "\x00"
                                                                        "\x00"

                                                                        "\x0B\x5E\xFE\xA7"sv}));

INSTANTIATE_TEST_SUITE_P(Test,
                         PostingsWriterFixture,
                         testing::Values(PostingsWriterCase{
                             .label_sets = {{{"job", "cron"}, {"server", "localhost"}}, {{"job", "cron"}, {"server", "127.0.0.1"}}},
                             .chunk_metadata_list = {{}, {}},
                             .expected = "\x00\x00\x00\x0C"
                                         "\x00\x00\x00\x02"
                                         "\x00\x00\x00\x04"
                                         "\x00\x00\x00\x05"
                                         "\x13\x45\xC5\x90"

                                         "\x00\x00\x00\x0C"
                                         "\x00\x00\x00\x02"
                                         "\x00\x00\x00\x04"
                                         "\x00\x00\x00\x05"
                                         "\x13\x45\xC5\x90"

                                         "\x00\x00\x00\x08"
                                         "\x00\x00\x00\x01"
                                         "\x00\x00\x00\x04"
                                         "\x73\xA3\x4A\x39"

                                         "\x00\x00\x00\x08"
                                         "\x00\x00\x00\x01"
                                         "\x00\x00\x00\x05"
                                         "\x81\xC8\xC9\x3A"

                                         "\x00\x00\x00\x39"
                                         "\x00\x00\x00\x04"
                                         "\x02"
                                         "\x00\x00\x00"
                                         "\x02"
                                         "\x03"
                                         "job"
                                         "\x04"
                                         "cron"
                                         "\x14"
                                         "\x02"
                                         "\x06"
                                         "server"
                                         "\x09"
                                         "127.0.0.1"
                                         "\x28"
                                         "\x02"
                                         "\x06"
                                         "server"
                                         "\x09"
                                         "localhost"
                                         "\x38"
                                         "\xF7\x79\x10\x67"sv}));

TEST_F(PostingsWriterFixture, PartialWrite) {
  // Arrange
  ChunkMetadataList chunk_metadata_list = {{}, {}};
  fill_data(
      LabelViewSetList{
          {{"server", "localhost"}},
          {{"server", "127.0.0.1"}},
      },
      chunk_metadata_list);
  PostingsWriter<QueryableEncodingBimap, decltype(stream_)> postings_writer{lss_, series_references_, stream_writer_};

  // Act
  postings_writer.write_postings(0);
  auto has_more_data_after_first_write = postings_writer.has_more_data();
  auto first_batch_data = stream_.str();
  stream_.str("");

  postings_writer.write_postings(1);
  auto has_more_data_after_second_write = postings_writer.has_more_data();
  auto second_batch_data = stream_.str();
  stream_.str("");

  postings_writer.write_postings(0);
  auto has_more_data_after_third_write = postings_writer.has_more_data();
  auto third_batch_data = stream_.str();
  stream_.str("");

  postings_writer.write_postings_table_offsets();

  // Assert
  EXPECT_TRUE(has_more_data_after_first_write);
  EXPECT_EQ(
      "\x00\x00\x00\x0C"
      "\x00\x00\x00\x02"
      "\x00\x00\x00\x03"
      "\x00\x00\x00\x04"
      "\x49\x58\x48\xD7"sv,
      first_batch_data);

  EXPECT_TRUE(has_more_data_after_second_write);
  EXPECT_EQ(
      "\x00\x00\x00\x08"
      "\x00\x00\x00\x01"
      "\x00\x00\x00\x03"
      "\xA7\x69\x2E\xD2"sv,
      second_batch_data);

  EXPECT_FALSE(has_more_data_after_third_write);
  EXPECT_EQ(
      "\x00\x00\x00\x08"
      "\x00\x00\x00\x01"
      "\x00\x00\x00\x04"
      "\x73\xA3\x4A\x39"sv,
      third_batch_data);

  EXPECT_EQ(
      "\x00\x00\x00\x2E"
      "\x00\x00\x00\x03"

      "\x02"
      "\x00\x00"
      "\x00"

      "\x02"
      "\x06"
      "server"
      "\x09"
      "127.0.0.1"
      "\x14"

      "\x02"
      "\x06"
      "server"
      "\x09"
      "localhost"
      "\x24"

      "\xAA\x6D\x56\x15"sv,
      stream_.view());
}

}  // namespace
