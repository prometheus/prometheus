#include <gtest/gtest.h>

#include "primitives/snug_composites.h"
#include "series_index/prometheus/tsdb/index/section_writer/label_indices_writer.h"
#include "series_index/prometheus/tsdb/index/section_writer/symbols_writer.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Prometheus::tsdb::index::StreamWriter;
using series_index::prometheus::tsdb::index::SymbolReferencesMap;
using series_index::prometheus::tsdb::index::section_writer::LabelIndicesWriter;
using series_index::prometheus::tsdb::index::section_writer::SymbolsWriter;
using std::operator""sv;

struct LabelIndicesWriterCase {
  std::vector<LabelViewSet> label_sets;
  std::string_view expected;
};

class LabelIndicesWriterFixture : public testing::TestWithParam<LabelIndicesWriterCase> {
 protected:
  using TrieIndex = series_index::TrieIndex<series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;
  using QueryableEncodingBimap = series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, TrieIndex>;

  std::ostringstream stream_;
  StreamWriter stream_writer_{stream_};
  QueryableEncodingBimap lss_;
  SymbolReferencesMap symbol_references_;
  LabelIndicesWriter<QueryableEncodingBimap> label_indices_writer{lss_, symbol_references_, stream_writer_};

  void SetUp() final {
    for (auto& label_set : GetParam().label_sets) {
      lss_.find_or_emplace(label_set);
    }

    SymbolsWriter<QueryableEncodingBimap>{lss_, symbol_references_, stream_writer_}.write();

    stream_.str("");
    stream_.clear();
  }
};

TEST_P(LabelIndicesWriterFixture, Test) {
  // Arrange

  // Act
  label_indices_writer.write_label_indices();
  label_indices_writer.write_label_indices_table();

  // Assert
  EXPECT_EQ(GetParam().expected, stream_.view());
}

INSTANTIATE_TEST_SUITE_P(EmptyLabelSet,
                         LabelIndicesWriterFixture,
                         testing::Values(LabelIndicesWriterCase{.label_sets = {},
                                                                .expected = "\x00\x00\x00\x04"
                                                                            "\x00\x00\x00\x00"
                                                                            "\x48\x67\x4B\xC7"sv}));

// INSTANTIATE_TEST_SUITE_P(LabelWithEmptyValue,
//                          LabelIndicesWriterFixture,
//                          testing::Values(LabelIndicesWriterCase{.label_sets = {{{"key", ""}}},
//                                                                 .expected = "\x00\x00\x00\x04"
//                                                                             "\x00\x00\x00\x00"
//                                                                             "\x48\x67\x4B\xC7"sv}));

INSTANTIATE_TEST_SUITE_P(Test,
                         LabelIndicesWriterFixture,
                         testing::Values(LabelIndicesWriterCase{
                             .label_sets = {{{"job", "cron"}, {"server", "localhost"}}, {{"job", "cron"}, {"server", "127.0.0.1"}}},
                             .expected = "\x00\x00\x00\x0C"
                                         "\x00\x00\x00\x01"
                                         "\x00\x00\x00\x01"
                                         "\x00\x00\x00\x02"
                                         "\x06\x74\x7C\x4E"

                                         "\x00\x00\x00\x10"
                                         "\x00\x00\x00\x01"
                                         "\x00\x00\x00\x02"
                                         "\x00\x00\x00\x01"
                                         "\x00\x00\x00\x04"
                                         "\x60\xB8\x80\x5D"

                                         "\x00\x00\x00\x13"
                                         "\x00\x00\x00\x02"
                                         "\x01"
                                         "\x03"
                                         "job"
                                         "\x00"

                                         "\x01"
                                         "\x06"
                                         "server"
                                         "\x14"
                                         "\xA8\xE9\x05\xC6"sv}));

}  // namespace