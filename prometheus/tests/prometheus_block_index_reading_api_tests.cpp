#include <filesystem>

#include <gtest/gtest.h>

#include "prometheus/block.h"

namespace {

struct BlockIndexReadingApi : public testing::Test {
  PromPP::Prometheus::Block::Timestamp block_min_time_stamp = 0;
  std::string test_file = "index_stub";

  std::vector<uint8_t> test_data = {
      0xba, 0xaa, 0xd7, 0x00, 0x02, 0x00, 0x00, 0x00, 0x0c, 0x00, 0x00, 0x00, 0x04, 0x01, 0x31, 0x01, 0x32, 0x01, 0x61, 0x01, 0x62, 0xa5, 0xa5, 0xa5, 0xa5,
      0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0c, 0x02, 0x02, 0x00, 0x03, 0x00, 0x02, 0x00, 0x01, 0x01, 0x01, 0x01, 0x02, 0xa5, 0xa5, 0xa5, 0xa5, 0x00,
      0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0c, 0x02, 0x02, 0x01, 0x03, 0x00, 0x02, 0x02, 0x01, 0x05, 0x02,
      0x03, 0x04, 0xa5, 0xa5, 0xa5, 0xa5, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xa5, 0xa5, 0xa5, 0xa5, 0x00, 0x00, 0x00,
      0x08, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0xa5, 0xa5, 0xa5, 0xa5, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x04,
      0xa5, 0xa5, 0xa5, 0xa5, 0x00, 0x00, 0x00, 0x0c, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x04, 0xa5, 0xa5, 0xa5, 0xa5, 0x00,
      0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0xa5, 0xa5, 0xa5, 0xa5, 0x00, 0x00, 0x00, 0x17, 0x00, 0x00, 0x00, 0x03, 0x02, 0x01, 0x61, 0x01, 0x31, 0x61,
      0x02, 0x01, 0x61, 0x01, 0x32, 0x71, 0x02, 0x01, 0x62, 0x01, 0x31, 0x81, 0x01, 0xa5, 0xa5, 0xa5, 0xa5, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05,
      0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x51, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x95, 0x00,
      0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x61, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xa1, 0xa5, 0xa5, 0xa5, 0xa5,
  };

  void SetUp() final {
    std::ofstream out(test_file, std::ios::binary);
    out.write(reinterpret_cast<const char*>(test_data.data()), test_data.size());
  }

  void TearDown() final { std::filesystem::remove(test_file.c_str()); }
};

TEST_F(BlockIndexReadingApi, ShouldCorrectlyReadSymbols) {
  FIO::Reader r(test_file);
  FIO::RegionReader rr1(r, r.file_size() - PromPP::Prometheus::Block::TableOfContent<FIO::RegionReader>::SIZE_OF_TABLE_OF_CONTENT_IN_BYTES);
  PromPP::Prometheus::Block::TableOfContent toc(rr1);

  FIO::RegionReader rr2(r, toc.symbols_table_offset());
  PromPP::Prometheus::Block::SymbolsTable st;
  PromPP::Prometheus::Block::read_symbols(rr2, [&st](const std::string_view& s) { st.emplace_back(s); });

  EXPECT_EQ(st.size(), 4);
  EXPECT_EQ(st[0], "1");
  EXPECT_EQ(st[1], "2");
  EXPECT_EQ(st[2], "a");
  EXPECT_EQ(st[3], "b");
}

TEST_F(BlockIndexReadingApi, ShouldCorrectlyReadSeriesLabelSet) {
  FIO::Reader r(test_file);
  FIO::RegionReader rr1(r, r.file_size() - PromPP::Prometheus::Block::TableOfContent<FIO::RegionReader>::SIZE_OF_TABLE_OF_CONTENT_IN_BYTES);
  PromPP::Prometheus::Block::TableOfContent toc(rr1);
  FIO::RegionReader rr2(r, toc.symbols_table_offset());
  PromPP::Prometheus::Block::SymbolsTable st;
  PromPP::Prometheus::Block::read_symbols(rr2, [&st](const std::string_view& s) { st.emplace_back(s); });

  FIO::RegionReader rr3(r, toc.series_table_offset(), toc.series_table_offset() + toc.series_table_length());
  rr3.align(4);
  [[maybe_unused]] const uint32_t series_len = rr3.get_varint<uint32_t>();
  PromPP::Prometheus::Block::LabelViewSet ls;
  PromPP::Prometheus::Block::read_label_set(rr3, st, ls);

  auto l1 = ls.begin();
  auto l2 = ls.begin() + 1;
  EXPECT_EQ(ls.size(), 2);
  EXPECT_EQ(l1->first, "a");
  EXPECT_EQ(l1->second, "1");
  EXPECT_EQ(l2->first, "b");
  EXPECT_EQ(l2->second, "1");
}

TEST_F(BlockIndexReadingApi, ShouldCorrectlyReadSeriesChunks) {
  FIO::Reader r(test_file);
  FIO::RegionReader rr1(r, r.file_size() - PromPP::Prometheus::Block::TableOfContent<FIO::RegionReader>::SIZE_OF_TABLE_OF_CONTENT_IN_BYTES);
  PromPP::Prometheus::Block::TableOfContent toc(rr1);

  FIO::RegionReader rr2(r, toc.series_table_offset(), toc.series_table_offset() + toc.series_table_length());
  rr2.align(4);
  [[maybe_unused]] const uint32_t series_len = rr2.get_varint<uint32_t>();
  [[maybe_unused]] const uint64_t labels_cnt = rr2.get_varint<uint64_t>();
  [[maybe_unused]] const uint32_t ln_id1 = rr2.get_varint<uint32_t>();
  [[maybe_unused]] const uint32_t lv_id1 = rr2.get_varint<uint32_t>();
  [[maybe_unused]] const uint32_t ln_id2 = rr2.get_varint<uint32_t>();
  [[maybe_unused]] const uint32_t lv_id2 = rr2.get_varint<uint32_t>();

  PromPP::Prometheus::Block::Chunks ch;
  PromPP::Prometheus::Block::read_chunks(rr2, block_min_time_stamp, ch);

  ASSERT_EQ(ch.size(), 2);
  auto chunk1 = ch.begin();
  auto chunk2 = ch.begin() + 1;
  EXPECT_EQ(std::get<0>(*chunk1), 0);
  EXPECT_EQ(std::get<1>(*chunk1), 1);
  EXPECT_EQ(std::get<2>(*chunk1), 1);
  EXPECT_EQ(std::get<0>(*chunk2), 2);
  EXPECT_EQ(std::get<1>(*chunk2), 3);
  EXPECT_EQ(std::get<2>(*chunk2), 2);
}

TEST_F(BlockIndexReadingApi, ShouldCorrectlyReadOneSeries) {
  FIO::Reader r(test_file);
  FIO::RegionReader rr1(r, r.file_size() - PromPP::Prometheus::Block::TableOfContent<FIO::RegionReader>::SIZE_OF_TABLE_OF_CONTENT_IN_BYTES);
  PromPP::Prometheus::Block::TableOfContent toc(rr1);
  FIO::RegionReader rr2(r, toc.symbols_table_offset());
  PromPP::Prometheus::Block::SymbolsTable st;
  PromPP::Prometheus::Block::read_symbols(rr2, [&st](const std::string_view& s) { st.emplace_back(s); });

  FIO::RegionReader rr3(r, toc.series_table_offset(), toc.series_table_offset() + toc.series_table_length());
  rr3.align(4);
  PromPP::Prometheus::Block::Series s;
  PromPP::Prometheus::Block::read_one_timeseries(rr3, st, block_min_time_stamp, s);

  auto l1 = s.label_set().begin();
  auto l2 = s.label_set().begin() + 1;
  EXPECT_EQ(s.label_set().size(), 2);
  EXPECT_EQ(l1->first, "a");
  EXPECT_EQ(l1->second, "1");
  EXPECT_EQ(l2->first, "b");
  EXPECT_EQ(l2->second, "1");

  ASSERT_EQ(s.chunks().size(), 2);
  auto chunk1 = s.chunks().begin();
  auto chunk2 = s.chunks().begin() + 1;
  EXPECT_EQ(std::get<0>(*chunk1), 0);
  EXPECT_EQ(std::get<1>(*chunk1), 1);
  EXPECT_EQ(std::get<2>(*chunk1), 1);
  EXPECT_EQ(std::get<0>(*chunk2), 2);
  EXPECT_EQ(std::get<1>(*chunk2), 3);
  EXPECT_EQ(std::get<2>(*chunk2), 2);
}

TEST_F(BlockIndexReadingApi, ShouldCorrectlyReadManySeries) {
  FIO::Reader r(test_file);
  FIO::RegionReader rr1(r, r.file_size() - PromPP::Prometheus::Block::TableOfContent<FIO::RegionReader>::SIZE_OF_TABLE_OF_CONTENT_IN_BYTES);
  PromPP::Prometheus::Block::TableOfContent toc(rr1);
  FIO::RegionReader rr2(r, toc.symbols_table_offset());
  PromPP::Prometheus::Block::SymbolsTable st;
  PromPP::Prometheus::Block::read_symbols(rr2, [&st](const std::string_view& s) { st.emplace_back(s); });

  FIO::RegionReader rr3(r, toc.series_table_offset(), toc.series_table_offset() + toc.series_table_length());
  std::vector<PromPP::Prometheus::Block::Series> index_series;
  PromPP::Prometheus::Block::read_many_timeseries(rr3, st, block_min_time_stamp,
                                                [&index_series](const PromPP::Prometheus::Block::Series& s) { index_series.push_back(s); });

  ASSERT_EQ(index_series.size(), 2);

  ASSERT_EQ(index_series[0].label_set().size(), 2);
  auto l12 = index_series[0].label_set().begin() + 1;
  auto l11 = index_series[0].label_set().begin();
  EXPECT_EQ(l11->first, "a");
  EXPECT_EQ(l11->second, "1");
  EXPECT_EQ(l12->first, "b");
  EXPECT_EQ(l12->second, "1");

  ASSERT_EQ(index_series[0].chunks().size(), 2);
  auto chunk11 = index_series[0].chunks().begin();
  auto chunk12 = index_series[0].chunks().begin() + 1;
  EXPECT_EQ(std::get<0>(*chunk11), 0);
  EXPECT_EQ(std::get<1>(*chunk11), 1);
  EXPECT_EQ(std::get<2>(*chunk11), 1);
  EXPECT_EQ(std::get<0>(*chunk12), 2);
  EXPECT_EQ(std::get<1>(*chunk12), 3);
  EXPECT_EQ(std::get<2>(*chunk12), 2);

  ASSERT_EQ(index_series[1].label_set().size(), 2);
  auto l21 = index_series[1].label_set().begin();
  auto l22 = index_series[1].label_set().begin() + 1;
  EXPECT_EQ(l21->first, "a");
  EXPECT_EQ(l21->second, "2");
  EXPECT_EQ(l22->first, "b");
  EXPECT_EQ(l22->second, "1");

  ASSERT_EQ(index_series[1].chunks().size(), 2);
  auto chunk21 = index_series[1].chunks().begin();
  auto chunk22 = index_series[1].chunks().begin() + 1;
  EXPECT_EQ(std::get<0>(*chunk21), 1);
  EXPECT_EQ(std::get<1>(*chunk21), 2);
  EXPECT_EQ(std::get<2>(*chunk21), 5);
  EXPECT_EQ(std::get<0>(*chunk22), 4);
  EXPECT_EQ(std::get<1>(*chunk22), 7);
  EXPECT_EQ(std::get<2>(*chunk22), 7);
}

TEST_F(BlockIndexReadingApi, ShouldThrowAnExceptionIfBlockMinTimestampIsLessThenOrEqualToAnySeriesChunksMinTimestamp) {
  using namespace PromPP::Prometheus::Block;  // NOLINT

  FIO::Reader r(test_file);
  FIO::RegionReader rr1(r, r.file_size() - PromPP::Prometheus::Block::TableOfContent<FIO::RegionReader>::SIZE_OF_TABLE_OF_CONTENT_IN_BYTES);
  TableOfContent toc(rr1);
  FIO::RegionReader rr2(r, toc.symbols_table_offset());
  SymbolsTable st;
  read_symbols(rr2, [&st](const std::string_view& s) { st.emplace_back(s); });

  FIO::RegionReader rr3(r, toc.series_table_offset(), toc.series_table_offset() + toc.series_table_length());
  ASSERT_THROW(read_many_timeseries(rr3, st, std::numeric_limits<PromPP::Prometheus::Block::Timestamp>::max(), [](const PromPP::Prometheus::Block::Series&) {}),
               std::runtime_error);
}

}  // namespace
