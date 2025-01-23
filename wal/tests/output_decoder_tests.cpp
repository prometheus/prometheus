#include <gtest/gtest.h>
#include <spanstream>

#include "bare_bones/type_traits.h"
#include "primitives/go_model.h"
#include "wal/encoder.h"
#include "wal/hashdex/go_model.h"
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
  std::vector<std::pair<Go::String, Go::String>> vector_external_labels_;
  Go::SliceView<std::pair<Go::String, Go::String>> external_labels_;

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
    Go::Slice<GoTimeSeries> go_time_series_slice;
    for (const GoTimeSeries& gts : gtss) {
      go_time_series_slice.push_back(gts);
    }
    Go::SliceView<Go::TimeSeries>* go_time_series_slice_view = reinterpret_cast<Go::SliceView<Go::TimeSeries>*>(&go_time_series_slice);
    PromPP::WAL::hashdex::GoModel hx;
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

    sr_.reset_to(rcts_);
  }
};

TEST_F(TestWALOutputDecoder, DumpEmptyData) {
  std::stringstream dump;
  std::stringstream segment_stream;
  stateless_relabeler_reset_to({{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2}});  // Keep
  OutputDecoder wod(sr_, output_lss_, external_labels_);
  Encoder enc{uint16_t{0}, uint8_t{0}};

  wod.dump_to(dump);

  EXPECT_EQ(0, dump.str().size());
}

TEST_F(TestWALOutputDecoder, DumpLoadSingleData) {
  std::stringstream dump;
  std::stringstream segment_stream;
  stateless_relabeler_reset_to({{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2}});  // Keep
  OutputDecoder wod(sr_, output_lss_, external_labels_);
  Encoder enc{uint16_t{0}, uint8_t{0}};

  make_segment({{{{"__name__", "value1"}, {"job", "abc"}}, {10, 1}}}, segment_stream, enc);
  segment_stream >> wod;
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
    segment_stream >> wod;
    wod.process_segment([&](LabelSetID ls_id [[maybe_unused]], Timestamp ts [[maybe_unused]], Sample::value_type v [[maybe_unused]]) {});
    wod.dump_to(dump);
  }

  {
    segment_stream.str("");
    make_segment({{{{"__name__", "value2"}, {"job", "abc"}}, {11, 1}}}, segment_stream, enc);
    segment_stream >> wod;
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
    segment_stream >> wod;
    wod.process_segment([&](LabelSetID ls_id [[maybe_unused]], Timestamp ts [[maybe_unused]], Sample::value_type v [[maybe_unused]]) {});
    wod.dump_to(dump);
  }

  wod.dump_to(dump);

  {
    segment_stream.str("");
    make_segment({{{{"__name__", "value2"}, {"job", "abc"}}, {11, 1}}}, segment_stream, enc);
    segment_stream >> wod;
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

  std::vector<std::vector<GoTimeSeries>> incoming_segments{{{{{"__name__", "value1"}, {"job", "abc"}}, {10, 1}}},  // keep
                                                           {},                                                     // load empty data
                                                           {
                                                               {{{"__name__", "value1"}, {"job", "abc"}}, {11, 1}},  // keep
                                                               {{{"__name__", "value2"}, {"job", "abc"}}, {11, 1}},  // keep
                                                           }};
  std::vector<RefSample> actual_ref_samples;
  actual_ref_samples.reserve(3);
  for (const auto& segment : incoming_segments) {
    segment_stream.str("");
    make_segment(segment, segment_stream, enc);
    segment_stream >> wod;
    wod.process_segment([&](LabelSetID ls_id, Timestamp ts, Sample::value_type v) { actual_ref_samples.emplace_back(ls_id, ts, v); });
  }

  std::vector<RefSample> expected_ref_samples{{.id = 0, .t = 10, .v = 1}, {.id = 0, .t = 11, .v = 1}, {.id = 1, .t = 11, .v = 1}};
  EXPECT_EQ(expected_ref_samples, actual_ref_samples);
}

TEST_F(TestWALOutputDecoder, ProcessSegmentWithDrop) {
  stateless_relabeler_reset_to({{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2}});  // Keep
  OutputDecoder wod(sr_, output_lss_, external_labels_);
  std::stringstream segment_stream;
  Encoder enc{uint16_t{0}, uint8_t{0}};

  std::vector<std::vector<GoTimeSeries>> incoming_segments{
      {{{{"__name__", "value1"}, {"job", "abc1"}}, {10, 1}}},  // drop
      {{{{"__name__", "value1"}, {"job", "abc"}}, {11, 1}}},   // keep
  };

  std::vector<RefSample> actual_ref_samples;
  actual_ref_samples.reserve(1);
  size_t processed{0};
  for (size_t i = 0; i < incoming_segments.size(); ++i) {
    segment_stream.str("");
    make_segment(incoming_segments[i], segment_stream, enc);
    segment_stream >> wod;
    wod.process_segment([&](LabelSetID ls_id, Timestamp ts, Sample::value_type v) {
      actual_ref_samples.emplace_back(ls_id, ts, v);
      ++processed;
    });
  }

  std::vector<RefSample> expected_ref_samples{{.id = 0, .t = 11, .v = 1}};
  EXPECT_EQ(expected_ref_samples, actual_ref_samples);
  EXPECT_EQ(1, processed);
}

TEST_F(TestWALOutputDecoder, ProcessSegmentWithDump) {
  stateless_relabeler_reset_to({{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2}});  // Keep
  OutputDecoder wod(sr_, output_lss_, external_labels_);
  std::stringstream segment_stream;
  std::stringstream dump;
  Encoder enc{uint16_t{0}, uint8_t{0}};

  std::vector<std::vector<GoTimeSeries>> incoming_segments{
      {{{{"__name__", "value1"}, {"job", "abc"}}, {10, 1}}},
      {},  // load empty data
      {{{{"__name__", "value1"}, {"job", "abc"}}, {11, 1}}, {{{"__name__", "value2"}, {"job", "abc"}}, {11, 1}}}};

  std::vector<RefSample> actual_ref_samples;
  actual_ref_samples.reserve(3);
  for (const auto& segment : incoming_segments) {
    segment_stream.str("");
    make_segment(segment, segment_stream, enc);
    segment_stream >> wod;
    wod.process_segment([&](LabelSetID ls_id, Timestamp ts, Sample::value_type v) { actual_ref_samples.emplace_back(ls_id, ts, v); });
    wod.dump_to(dump);
  }

  std::vector<RefSample> expected_ref_samples{{.id = 0, .t = 10, .v = 1}, {.id = 0, .t = 11, .v = 1}, {.id = 1, .t = 11, .v = 1}};
  EXPECT_EQ(expected_ref_samples, actual_ref_samples);

  SnugComposites::LabelSet::EncodingBimap output_lss2;
  OutputDecoder wod2(sr_, output_lss2, external_labels_);
  wod2.load_from(dump);
  Encoder enc2{uint16_t{0}, uint8_t{0}};

  std::vector<std::vector<GoTimeSeries>> incoming_segments_2{
      {{{{"__name__", "value1"}, {"job", "abc"}}, {10, 1}}},
      {},  // load empty data
      {{{{"__name__", "value1"}, {"job", "abc"}}, {11, 1}}, {{{"__name__", "value2"}, {"job", "abc"}}, {11, 1}}},
      {{{{"__name__", "value1"}, {"job", "abc"}}, {12, 1}},
       {{{"__name__", "value2"}, {"job", "abc"}}, {12, 1}},
       {{{"__name__", "value3"}, {"job", "abc"}}, {12, 1}}}};

  actual_ref_samples.clear();
  actual_ref_samples.reserve(6);
  for (const auto& segment : incoming_segments_2) {
    segment_stream.str("");
    make_segment(segment, segment_stream, enc2);
    segment_stream >> wod2;
    wod2.process_segment([&](LabelSetID ls_id, Timestamp ts, Sample::value_type v) { actual_ref_samples.emplace_back(ls_id, ts, v); });
  }

  std::vector<RefSample> expected_ref_samples_2{{.id = 0, .t = 10, .v = 1}, {.id = 0, .t = 11, .v = 1}, {.id = 1, .t = 11, .v = 1},
                                                {.id = 0, .t = 12, .v = 1}, {.id = 1, .t = 12, .v = 1}, {.id = 2, .t = 12, .v = 1}};
  EXPECT_EQ(expected_ref_samples_2, actual_ref_samples);
}

//
// ProtobufEncoder
//

struct TestProtobufEncoder : public testing::Test {};

TEST_F(TestProtobufEncoder, Encode) {
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap output_lss0;
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap output_lss1;
  std::vector<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap*> output_lsses{&output_lss0, &output_lss1};

  std::vector<PromPP::WAL::RefSample> ref_samples0;
  ref_samples0.emplace_back(output_lss0.find_or_emplace(LabelViewSet{{"__name__", "value1"}, {"job", "abc"}}), 10, 1);
  ref_samples0.emplace_back(output_lss0.find_or_emplace(LabelViewSet{{"__name__", "value1"}, {"job", "abc"}}), 9, 2);
  ref_samples0.emplace_back(output_lss0.find_or_emplace(LabelViewSet{{"__name__", "value2"}, {"job", "abc"}}), 10, 1);
  ShardRefSample srs0;
  srs0.ref_samples.reset_to(ref_samples0.data(), ref_samples0.size());
  srs0.shard_id = 0;

  std::vector<PromPP::WAL::RefSample> ref_samples1;
  ref_samples1.emplace_back(output_lss1.find_or_emplace(LabelViewSet{{"__name__", "value3"}, {"job", "abc3"}}), 10, 1);
  ShardRefSample srs1;
  srs1.ref_samples.reset_to(ref_samples1.data(), ref_samples1.size());
  srs1.shard_id = 1;

  std::vector<ShardRefSample*> vector_batch{&srs0, &srs1};
  Go::SliceView<ShardRefSample*> batch;
  batch.reset_to(vector_batch.data(), vector_batch.size());

  ProtobufEncoder penc(std::move(output_lsses));
  Go::Slice<Go::Slice<char>> out_slices;
  out_slices.resize(2);
  penc.encode(batch, out_slices);

  std::vector<int8_t> expected_proto1{10, 58, 10,  18, 10,  8,   95, 95, 110, 97, 109, 101, 95,  95,  18,  6,  118, 97, 108, 117, 101, 49,
                                      10, 10, 10,  3,  106, 111, 98, 18, 3,   97, 98,  99,  18,  11,  9,   0,  0,   0,  0,   0,   0,   0,
                                      64, 16, 9,   18, 11,  9,   0,  0,  0,   0,  0,   0,   -16, 63,  16,  10, 10,  46, 10,  18,  10,  8,
                                      95, 95, 110, 97, 109, 101, 95, 95, 18,  6,  118, 97,  108, 117, 101, 51, 10,  11, 10,  3,   106, 111,
                                      98, 18, 4,   97, 98,  99,  51, 18, 11,  9,  0,   0,   0,   0,   0,   0,  -16, 63, 16,  10};
  std::vector<int8_t> expected_proto2{10, 45, 10,  18,  10, 8,  95, 95, 110, 97, 109, 101, 95, 95, 18, 6, 118, 97, 108, 117, 101, 50, 10, 10,
                                      10, 3,  106, 111, 98, 18, 3,  97, 98,  99, 18,  11,  9,  0,  0,  0, 0,   0,  0,   -16, 63,  16, 10};

  std::string proto1;
  bool ok = snappy::Uncompress(out_slices[0].data(), out_slices[0].size(), &proto1);
  EXPECT_TRUE(ok);
  EXPECT_EQ(expected_proto1, std::vector<int8_t>(proto1.begin(), proto1.end()));

  std::string proto2;
  ok = snappy::Uncompress(out_slices[1].data(), out_slices[1].size(), &proto2);
  EXPECT_TRUE(ok);
  EXPECT_EQ(expected_proto2, std::vector<int8_t>(proto2.begin(), proto2.end()));
}

}  // namespace
