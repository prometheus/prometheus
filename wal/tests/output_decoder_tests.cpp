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

// PROMPP_ALWAYS_INLINE LabelViewSet make_label_set(std::initializer_list<LabelView> lvs) {
//   LabelViewSet labels;
//   for (const LabelView& lv : lvs) {
//     labels.add(lv);
//   }

//   return labels;
// }

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
  PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> external_labels_;

  void SetUp() final {
    // external_labels
    external_labels_.reset_to(vector_external_labels_.data(), vector_external_labels_.size());
  }
};

TEST_F(TestWALOutputDecoder, SingleDumpLoad) {
  // make Hashdex
  PromPP::Primitives::Go::Slice<GoTimeSeries> go_time_series_slice;
  go_time_series_slice.push_back(GoTimeSeries({{"__name__", "value1"}, {"job", "abc"}}, {10, 1}));
  PromPP::Primitives::Go::SliceView<PromPP::Primitives::Go::TimeSeries>* go_time_series_slice_view =
      reinterpret_cast<PromPP::Primitives::Go::SliceView<PromPP::Primitives::Go::TimeSeries>*>(&go_time_series_slice);
  PromPP::WAL::GoModelHashdex hx;
  hx.presharding(*go_time_series_slice_view);

  // make Segment from encoder
  PromPP::WAL::Encoder enc{uint16_t{0}, uint8_t{0}};
  EncodeStatistic stats;
  enc.add(hx, &stats);
  std::stringstream segment_stream;
  enc.finalize(&stats, segment_stream);

  // make StatelessRelabeler
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts{&rct};
  Relabel::StatelessRelabeler sr(rcts);

  WALOutputDecoder wod(sr, external_labels_);
  std::ispanstream inspan(std::string_view(segment_stream.str().data(), segment_stream.str().size()));
  inspan >> wod;
  wod.process_segment([&](LabelSetID ls_id [[maybe_unused]], Timestamp ts [[maybe_unused]], Sample::value_type v [[maybe_unused]]) {});

  std::stringstream ss;
  wod.dump_to(ss);

  WALOutputDecoder wod2(sr, external_labels_);
  wod2.load_from(ss);

  EXPECT_EQ(wod.cache(), wod2.cache());
  EXPECT_EQ(wod.output_lss().size(), wod2.output_lss().size());
  for (size_t i = 0; i < wod.output_lss().size(); ++i) {
    EXPECT_EQ(wod.output_lss()[i], wod2.output_lss()[i]);
  }
}

TEST_F(TestWALOutputDecoder, Emplace) {
  // make Hashdex
  PromPP::Primitives::Go::Slice<GoTimeSeries> go_time_series_slice;
  go_time_series_slice.push_back(GoTimeSeries({{"__name__", "value1"}, {"job", "abc"}}, {10, 1}));
  PromPP::Primitives::Go::SliceView<PromPP::Primitives::Go::TimeSeries>* go_time_series_slice_view =
      reinterpret_cast<PromPP::Primitives::Go::SliceView<PromPP::Primitives::Go::TimeSeries>*>(&go_time_series_slice);
  PromPP::WAL::GoModelHashdex hx;
  hx.presharding(*go_time_series_slice_view);

  // make Segment from encoder
  PromPP::WAL::Encoder enc{uint16_t{0}, uint8_t{0}};
  EncodeStatistic stats;
  enc.add(hx, &stats);
  std::stringstream segment_stream;
  enc.finalize(&stats, segment_stream);

  // make StatelessRelabeler
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts{&rct};
  Relabel::StatelessRelabeler sr(rcts);

  WALOutputDecoder wod(sr, external_labels_);
  std::ispanstream inspan(std::string_view(segment_stream.str().data(), segment_stream.str().size()));
  inspan >> wod;
  wod.process_segment([&](LabelSetID ls_id, Timestamp ts, Sample::value_type v) {
    std::cout << "process_segment ls_id: " << ls_id << " ts " << ts << " v " << v << std::endl;
  });

  // enc.add(hx, &stats);
  // segment_stream.str("");
  // enc.finalize(&stats, segment_stream);
  // std::ispanstream inspan2(std::string_view(segment_stream.str().data(), segment_stream.str().size()));
  // inspan2 >> wod;
  // wod.process_segment([&](LabelSetID ls_id, Timestamp ts, Sample::value_type v) {
  //   std::cout << "process_segment ls_id: " << ls_id << " ts " << ts << " v " << v << std::endl;
  // });
}

}  // namespace
