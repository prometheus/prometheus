#include <gtest/gtest.h>

#include "primitives/snug_composites.h"
#include "series_index/prometheus/tsdb/index/section_writer/symbols_writer.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Prometheus::tsdb::index::StreamWriter;
using series_index::prometheus::tsdb::index::SymbolReferencesMap;
using series_index::prometheus::tsdb::index::section_writer::SymbolsWriter;
using std::operator""sv;

struct SymbolsWriterCase {
  std::vector<LabelViewSet> label_sets;
  std::string_view expected;
};

class SymbolsWriterFixture : public testing::TestWithParam<SymbolsWriterCase> {
 protected:
  using TrieIndex = series_index::TrieIndex<series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;
  using QueryableEncodingBimap = series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, TrieIndex>;

  std::ostringstream stream_;
  StreamWriter stream_writer_{stream_};
  QueryableEncodingBimap lss_;
  SymbolReferencesMap symbol_references_;
  SymbolsWriter<QueryableEncodingBimap> symbols_writer_{lss_, symbol_references_, stream_writer_};

  void SetUp() final {
    for (auto& label_set : GetParam().label_sets) {
      lss_.find_or_emplace(label_set);
    }
  }
};

TEST_P(SymbolsWriterFixture, Test) {
  // Arrange

  // Act
  symbols_writer_.write();

  // Assert
  EXPECT_EQ(GetParam().expected, stream_.view());
}

INSTANTIATE_TEST_SUITE_P(EmptyLabelSet,
                         SymbolsWriterFixture,
                         testing::Values(SymbolsWriterCase{.label_sets = {},
                                                           .expected = "\x00\x00\x00\x05"
                                                                       "\x00\x00\x00\x01"
                                                                       "\x0"
                                                                       "\x56\xD0\xEE\x42"sv}));
INSTANTIATE_TEST_SUITE_P(LabelWithEmptyValue,
                         SymbolsWriterFixture,
                         testing::Values(SymbolsWriterCase{.label_sets = {{{"key", ""}}},
                                                           .expected = "\x00\x00\x00\x09"
                                                                       "\x00\x00\x00\x02"
                                                                       "\x0"
                                                                       "\x03"
                                                                       "key"
                                                                       "\x22\x8B\x97\x4E"sv}));

INSTANTIATE_TEST_SUITE_P(TestUniquenessAndSorting,
                         SymbolsWriterFixture,
                         testing::Values(SymbolsWriterCase{
                             .label_sets = {{{"job", "cron"}, {"server", "localhost"}}, {{"job", "cron"}, {"server", "127.0.0.1"}}},
                             .expected = "\x00\x00\x00\x29"
                                         "\x00\x00\x00\x06"
                                         "\x00"
                                         "\x09"
                                         "127.0.0.1"
                                         "\x04"
                                         "cron"
                                         "\x03"
                                         "job"
                                         "\x09"
                                         "localhost"
                                         "\x06"
                                         "server"
                                         "\xCB\xE1\x54\x24"sv}));

}  // namespace
