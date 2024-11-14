#include <gtest/gtest.h>
#include <spanstream>

#include "bare_bones/type_traits.h"
#include "primitives/go_model.h"
#include "wal/encoder.h"
#include "wal/hashdex.h"
#include "wal/output_decoder.h"

struct GoLabelSet {
  PromPP::Primitives::Go::Slice<char> data;
  PromPP::Primitives::Go::Slice<PromPP::Primitives::Go::LabelView> pairs;
};

struct GoTimeSeries {
  GoLabelSet label_set;
  uint64_t timestamp;
  double value;

  PROMPP_ALWAYS_INLINE GoTimeSeries() {}
  PROMPP_ALWAYS_INLINE GoTimeSeries(std::initializer_list<PromPP::Primitives::LabelView> lvs, const PromPP::Primitives::Sample& sample) {
    size_t index{0};
    for (const auto& [ln, lv] : lvs) {
      PromPP::Primitives::Go::LabelView go_label_view;
      label_set.data.push_back(ln.begin(), ln.end());
      label_set.data.push_back(':');
      go_label_view.name = {static_cast<uint32_t>(index), static_cast<uint32_t>(ln.size())};
      index += ln.size() + 1;
      label_set.data.push_back(lv.begin(), lv.end());
      label_set.data.push_back(';');
      go_label_view.value = {static_cast<uint32_t>(index), static_cast<uint32_t>(lv.size())};
      index += lv.size() + 1;
      label_set.pairs.push_back(go_label_view);
    }
    timestamp = static_cast<uint64_t>(sample.timestamp());
    value = sample.value();
  }
};

template <>
struct BareBones::IsTriviallyReallocatable<GoTimeSeries> : std::true_type {};

namespace {

using namespace PromPP::WAL;         // NOLINT
using namespace PromPP::Primitives;  // NOLINT
using namespace PromPP::Prometheus;  // NOLINT

struct EncodeStatistic {
  uint32_t samples;
  uint32_t series;
  int64_t earliest_timestamp;
  int64_t latest_timestamp;
  uint32_t remainder_size;
};

struct RelabelConfigTest {
  std::vector<std::string_view> source_labels{};
  std::string_view separator{};
  std::string_view regex{};
  uint64_t modulus{0};
  std::string_view target_label{};
  std::string_view replacement{};
  uint8_t action{0};
};

struct TestWALOutputDecoder : public testing::Test {
  // external_labels
  std::vector<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> vector_external_labels_;
  Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> external_labels_;

  // Encoder
  EncodeStatistic stats_;

  // StatelessRelabeler
  std::vector<RelabelConfigTest> rcts_buf_{{.source_labels = std::vector<std::string_view>{"__name__"}, .regex = ".*", .action = 2}};  // Keep
  std::vector<RelabelConfigTest*> rcts_{&rcts_buf_[0]};
  Relabel::StatelessRelabeler sr_{rcts_};

  // Output LSS
  SnugComposites::LabelSet::EncodingBimap output_lss_;

  void SetUp() final {
    // external_labels
    external_labels_.reset_to(vector_external_labels_.data(), vector_external_labels_.size());
  }

  template <class SegmentStream>
  PROMPP_ALWAYS_INLINE void make_segment(const std::vector<GoTimeSeries>& gtss, SegmentStream& segment_stream, Encoder& enc) {
    // make Hashdex
    PromPP::Primitives::Go::Slice<GoTimeSeries> go_time_series_slice;
    for (const GoTimeSeries& gts : gtss) {
      go_time_series_slice.push_back(gts);
    }
    PromPP::Primitives::Go::SliceView<PromPP::Primitives::Go::TimeSeries>* go_time_series_slice_view =
        reinterpret_cast<PromPP::Primitives::Go::SliceView<PromPP::Primitives::Go::TimeSeries>*>(&go_time_series_slice);
    PromPP::WAL::GoModelHashdex hx;
    hx.presharding(*go_time_series_slice_view);

    // make Segment from encoder
    enc.add(hx, &stats_);
    enc.finalize(&stats_, segment_stream);
  }

  PROMPP_ALWAYS_INLINE void stateless_relabeler_reset_to(std::initializer_list<RelabelConfigTest> cfgs) {
    rcts_.resize(0);
    rcts_buf_.resize(0);
    rcts_.reserve(cfgs.size());
    rcts_buf_.reserve(cfgs.size());
    for (const RelabelConfigTest& cfg : cfgs) {
      rcts_buf_.push_back(cfg);
    }
    for (RelabelConfigTest& cfg : rcts_buf_) {
      rcts_.push_back(&cfg);
    }
  }
};

TEST_F(TestWALOutputDecoder, DumpLoadSingleData) {
  std::stringstream dump;
  std::stringstream segment_stream;
  stateless_relabeler_reset_to({{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2}});  // Keep
  OutputDecoder wod(sr_, output_lss_, external_labels_);
  Encoder enc{uint16_t{0}, uint8_t{0}};

  make_segment({{{{"__name__", "value1"}, {"job", "abc"}}, {10, 1}}}, segment_stream, enc);
  std::ispanstream inspan(segment_stream.view());
  inspan >> wod;
  wod.process_segment([&](LabelSetID ls_id [[maybe_unused]], Timestamp ts [[maybe_unused]], Sample::value_type v [[maybe_unused]]) {});

  wod.dump_to(dump);

  SnugComposites::LabelSet::EncodingBimap output_lss2;
  OutputDecoder wod2(sr_, output_lss2, external_labels_);
  wod2.load_from(dump);

  EXPECT_EQ(1, wod.cache().size());
  EXPECT_EQ(wod.cache(), wod2.cache());
  EXPECT_EQ(output_lss_.size(), output_lss2.size());
  for (size_t i = 0; i < output_lss_.size(); ++i) {
    EXPECT_EQ(output_lss_[i], output_lss2[i]);
  }
}

TEST_F(TestWALOutputDecoder, DumpLoadDoubleData) {
  std::stringstream dump;
  std::stringstream segment_stream;
  stateless_relabeler_reset_to({{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2}});  // Keep
  OutputDecoder wod(sr_, output_lss_, external_labels_);
  Encoder enc{uint16_t{0}, uint8_t{0}};

  {
    make_segment({{{{"__name__", "value1"}, {"job", "abc"}}, {10, 1}}}, segment_stream, enc);
    std::ispanstream inspan(segment_stream.view());
    inspan >> wod;
    wod.process_segment([&](LabelSetID ls_id [[maybe_unused]], Timestamp ts [[maybe_unused]], Sample::value_type v [[maybe_unused]]) {});
    wod.dump_to(dump);
  }

  {
    segment_stream.str("");
    make_segment({{{{"__name__", "value2"}, {"job", "abc"}}, {11, 1}}}, segment_stream, enc);
    std::ispanstream inspan(segment_stream.view());
    inspan >> wod;
    wod.process_segment([&](LabelSetID ls_id [[maybe_unused]], Timestamp ts [[maybe_unused]], Sample::value_type v [[maybe_unused]]) {});
    wod.dump_to(dump);
  }

  SnugComposites::LabelSet::EncodingBimap output_lss2;
  OutputDecoder wod2(sr_, output_lss2, external_labels_);
  wod2.load_from(dump);

  EXPECT_EQ(2, wod.cache().size());
  EXPECT_EQ(wod.cache(), wod2.cache());
  EXPECT_EQ(output_lss_.size(), output_lss2.size());
  for (size_t i = 0; i < output_lss_.size(); ++i) {
    EXPECT_EQ(output_lss_[i], output_lss2[i]);
  }
}

TEST_F(TestWALOutputDecoder, DumpLoadDataEmptyData) {
  std::stringstream dump;
  std::stringstream segment_stream;
  stateless_relabeler_reset_to({{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2}});  // Keep
  OutputDecoder wod(sr_, output_lss_, external_labels_);
  Encoder enc{uint16_t{0}, uint8_t{0}};

  {
    make_segment({{{{"__name__", "value1"}, {"job", "abc"}}, {10, 1}}}, segment_stream, enc);
    std::ispanstream inspan(segment_stream.view());
    inspan >> wod;
    wod.process_segment([&](LabelSetID ls_id [[maybe_unused]], Timestamp ts [[maybe_unused]], Sample::value_type v [[maybe_unused]]) {});
    wod.dump_to(dump);
  }

  wod.dump_to(dump);

  {
    segment_stream.str("");
    make_segment({{{{"__name__", "value2"}, {"job", "abc"}}, {11, 1}}}, segment_stream, enc);
    std::ispanstream inspan(segment_stream.view());
    inspan >> wod;
    wod.process_segment([&](LabelSetID ls_id [[maybe_unused]], Timestamp ts [[maybe_unused]], Sample::value_type v [[maybe_unused]]) {});
    wod.dump_to(dump);
  }

  SnugComposites::LabelSet::EncodingBimap output_lss2;
  OutputDecoder wod2(sr_, output_lss2, external_labels_);
  wod2.load_from(dump);

  EXPECT_EQ(2, wod.cache().size());
  EXPECT_EQ(wod.cache(), wod2.cache());
  EXPECT_EQ(output_lss_.size(), output_lss2.size());
  for (size_t i = 0; i < output_lss_.size(); ++i) {
    EXPECT_EQ(output_lss_[i], output_lss2[i]);
  }
}

TEST_F(TestWALOutputDecoder, ProcessSegment) {
  stateless_relabeler_reset_to({{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2}});  // Keep
  OutputDecoder wod(sr_, output_lss_, external_labels_);
  std::stringstream segment_stream;
  Encoder enc{uint16_t{0}, uint8_t{0}};

  std::vector<std::vector<GoTimeSeries>> expected_segments{
      {{{{"__name__", "value1"}, {"job", "abc"}}, {10, 1}}},
      {},  // load empty data
      {{{{"__name__", "value1"}, {"job", "abc"}}, {11, 1}}, {{{"__name__", "value2"}, {"job", "abc"}}, {11, 1}}}};

  for (const auto& expected_segment : expected_segments) {
    segment_stream.str("");
    make_segment(expected_segment, segment_stream, enc);
    std::ispanstream inspan(segment_stream.view());
    inspan >> wod;
    wod.process_segment([&](LabelSetID ls_id, Timestamp ts, Sample::value_type v) {
      EXPECT_LT(ls_id, expected_segment.size());
      EXPECT_EQ(expected_segment[ls_id].timestamp, ts);
      EXPECT_EQ(std::bit_cast<uint64_t>(expected_segment[ls_id].value), std::bit_cast<uint64_t>(v));
    });
  }
}

TEST_F(TestWALOutputDecoder, ProcessSegmentWithDump) {
  stateless_relabeler_reset_to({{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2}});  // Keep
  OutputDecoder wod(sr_, output_lss_, external_labels_);
  std::stringstream segment_stream;
  std::stringstream dump;
  Encoder enc{uint16_t{0}, uint8_t{0}};

  std::vector<std::vector<GoTimeSeries>> expected_segments{
      {{{{"__name__", "value1"}, {"job", "abc"}}, {10, 1}}},
      {},  // load empty data
      {{{{"__name__", "value1"}, {"job", "abc"}}, {11, 1}}, {{{"__name__", "value2"}, {"job", "abc"}}, {11, 1}}}};

  for (const auto& expected_segment : expected_segments) {
    segment_stream.str("");
    make_segment(expected_segment, segment_stream, enc);
    std::ispanstream inspan(segment_stream.view());
    inspan >> wod;
    wod.process_segment([&](LabelSetID ls_id, Timestamp ts, Sample::value_type v) {
      EXPECT_LT(ls_id, expected_segment.size());
      EXPECT_EQ(expected_segment[ls_id].timestamp, ts);
      EXPECT_EQ(std::bit_cast<uint64_t>(expected_segment[ls_id].value), std::bit_cast<uint64_t>(v));
    });
    wod.dump_to(dump);
  }

  SnugComposites::LabelSet::EncodingBimap output_lss2;
  OutputDecoder wod2(sr_, output_lss2, external_labels_);
  wod2.load_from(dump);
  Encoder enc2{uint16_t{0}, uint8_t{0}};

  std::vector<std::vector<GoTimeSeries>> expected_segments2{
      {{{{"__name__", "value1"}, {"job", "abc"}}, {10, 1}}},
      {},  // load empty data
      {{{{"__name__", "value1"}, {"job", "abc"}}, {11, 1}}, {{{"__name__", "value2"}, {"job", "abc"}}, {11, 1}}},
      {{{{"__name__", "value1"}, {"job", "abc"}}, {12, 1}},
       {{{"__name__", "value2"}, {"job", "abc"}}, {12, 1}},
       {{{"__name__", "value3"}, {"job", "abc"}}, {12, 1}}}};

  for (const auto& expected_segment : expected_segments2) {
    segment_stream.str("");
    make_segment(expected_segment, segment_stream, enc2);
    std::ispanstream inspan(segment_stream.view());
    inspan >> wod2;
    wod2.process_segment([&](LabelSetID ls_id, Timestamp ts, Sample::value_type v) {
      EXPECT_LT(ls_id, expected_segment.size());
      EXPECT_EQ(expected_segment[ls_id].timestamp, ts);
      EXPECT_EQ(std::bit_cast<uint64_t>(expected_segment[ls_id].value), std::bit_cast<uint64_t>(v));
    });
  }
}

}  // namespace
