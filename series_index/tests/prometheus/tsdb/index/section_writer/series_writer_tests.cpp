#include <gtest/gtest.h>

#include "primitives/snug_composites.h"
#include "series_index/prometheus/tsdb/index/section_writer/series_writer.h"
#include "series_index/prometheus/tsdb/index/section_writer/symbols_writer.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Prometheus::tsdb::index::StreamWriter;
using series_index::prometheus::tsdb::index::SeriesReferencesMap;
using series_index::prometheus::tsdb::index::SymbolReferencesMap;
using series_index::prometheus::tsdb::index::section_writer::SeriesWriter;
using series_index::prometheus::tsdb::index::section_writer::SymbolsWriter;
using std::operator""sv;

struct SeriesWriterCase {
  std::vector<LabelViewSet> label_sets;
  std::string_view expected;
};

class SeriesWriterFixture : public testing::TestWithParam<SeriesWriterCase> {
 protected:
  using TrieIndex = series_index::TrieIndex<series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;
  using QueryableEncodingBimap = series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, TrieIndex>;

  std::ostringstream stream_;
  StreamWriter stream_writer_{stream_};
  QueryableEncodingBimap lss_;
  SymbolReferencesMap symbol_references_;
  SeriesReferencesMap series_references_;
  SeriesWriter<QueryableEncodingBimap> series_writer_{lss_, symbol_references_, series_references_, stream_writer_};

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

INSTANTIATE_TEST_SUITE_P(EmptyLabelSet, SeriesWriterFixture, testing::Values(SeriesWriterCase{.label_sets = {}, .expected = ""}));

// INSTANTIATE_TEST_SUITE_P(GG, SeriesWriterFixture, testing::Values(SeriesWriterCase{.label_sets = {}, .expected = ""}));

INSTANTIATE_TEST_SUITE_P(Test,
                         SeriesWriterFixture,
                         testing::Values(SeriesWriterCase{
                             .label_sets = {{{"job", "cron"}, {"server", "localhost"}}, {{"job", "cron"}, {"server", "127.0.0.1"}}},
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

}  // namespace