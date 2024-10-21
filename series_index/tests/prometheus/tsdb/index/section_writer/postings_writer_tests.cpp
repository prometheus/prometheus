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

  void reset_stream() noexcept {
    stream_.str("");
    stream_writer_.writer().set_stream(&stream_);
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
                                                                        "\x48\x67\x4B\xC7"sv}));

INSTANTIATE_TEST_SUITE_P(Test,
                         PostingsWriterFixture,
                         testing::Values(PostingsWriterCase{.label_sets =
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
                                                            .expected = "\x00\x00\x00\x0C"
                                                                        "\x00\x00\x00\x02"
                                                                        "\x00\x00\x00\x04"
                                                                        "\x00\x00\x00\x06"
                                                                        "\x00\x15\x36\x64"

                                                                        "\x00\x00\x00\x0C"
                                                                        "\x00\x00\x00\x02"
                                                                        "\x00\x00\x00\x04"
                                                                        "\x00\x00\x00\x06"
                                                                        "\x00\x15\x36\x64"

                                                                        "\x00\x00\x00\x08"
                                                                        "\x00\x00\x00\x01"
                                                                        "\x00\x00\x00\x04"
                                                                        "\x73\xA3\x4A\x39"

                                                                        "\x00\x00\x00\x08"
                                                                        "\x00\x00\x00\x01"
                                                                        "\x00\x00\x00\x06"
                                                                        "\x92\x98\x3A\xCE"

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
  ChunkMetadataList chunk_metadata_list = {
      {{.min_timestamp = 1000, .max_timestamp = 2001, .reference = 0},
       {.min_timestamp = 2002, .max_timestamp = 4004, .reference = 100},
       {.min_timestamp = 4005, .max_timestamp = 5005, .reference = 125}},
      {},
      {{.min_timestamp = 1000, .max_timestamp = 2001, .reference = 0},
       {.min_timestamp = 2002, .max_timestamp = 4004, .reference = 100},
       {.min_timestamp = 4005, .max_timestamp = 5005, .reference = 125}},
  };
  fill_data(
      LabelViewSetList{
          {{"job", "cron"}, {"server", "localhost"}},
          {{"job", "cron"}, {"server", "remote"}},
          {{"job", "cron"}, {"server", "127.0.0.1"}},
      },
      chunk_metadata_list);
  PostingsWriter postings_writer{lss_, series_references_, stream_writer_};

  // Act
  postings_writer.write_postings(0);
  auto has_more_data_after_first_write = postings_writer.has_more_data();
  auto first_batch_data = stream_.str();
  reset_stream();

  postings_writer.write_postings(1);
  auto has_more_data_after_second_write = postings_writer.has_more_data();
  auto second_batch_data = stream_.str();
  reset_stream();

  postings_writer.write_postings(17);
  auto has_more_data_after_third_write = postings_writer.has_more_data();
  auto third_batch_data = stream_.str();
  reset_stream();

  postings_writer.write_postings_table_offsets();

  // Assert
  EXPECT_TRUE(has_more_data_after_first_write);
  EXPECT_EQ(
      "\x00\x00\x00\x0C"
      "\x00\x00\x00\x02"
      "\x00\x00\x00\x04"
      "\x00\x00\x00\x06"
      "\x00\x15\x36\x64"sv,
      first_batch_data);

  EXPECT_TRUE(has_more_data_after_second_write);
  EXPECT_EQ(
      "\x00\x00\x00\x0C"
      "\x00\x00\x00\x02"
      "\x00\x00\x00\x04"
      "\x00\x00\x00\x06"
      "\x00\x15\x36\x64"sv,
      second_batch_data);

  EXPECT_TRUE(has_more_data_after_third_write);
  EXPECT_EQ(
      "\x00\x00\x00\x08"
      "\x00\x00\x00\x01"
      "\x00\x00\x00\x04"
      "\x73\xA3\x4A\x39"
      "\x00\x00\x00\x08"
      "\x00\x00\x00\x01"
      "\x00\x00\x00\x06"
      "\x92\x98\x3A\xCE"sv,
      third_batch_data);

  EXPECT_EQ(
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
      "\xF7\x79\x10\x67"sv,
      stream_.view());
}

}  // namespace
