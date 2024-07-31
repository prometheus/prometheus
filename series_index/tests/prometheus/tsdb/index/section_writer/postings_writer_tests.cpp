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
  StreamWriter stream_writer_{&stream_};
  QueryableEncodingBimap lss_;
  SymbolReferencesMap symbol_references_;
  SeriesReferencesMap series_references_;
  PostingsWriter<QueryableEncodingBimap> postings_writer{lss_, series_references_, stream_writer_};

  void SetUp() final {
    for (auto& label_set : GetParam().label_sets) {
      lss_.find_or_emplace(label_set);
    }

    std::ostringstream stream;
    StreamWriter stream_writer{&stream};
    SymbolsWriter<QueryableEncodingBimap>{lss_, symbol_references_, stream_writer}.write();
    SeriesWriter<QueryableEncodingBimap, ChunkMetadataList>{lss_, GetParam().chunk_metadata_list, symbol_references_, series_references_}.write(stream_writer);
  }
};

TEST_P(PostingsWriterFixture, Test) {
  // Arrange

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
                         testing::Values(PostingsWriterCase{.label_sets = {{{"key", ""}}},
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

}  // namespace