#include <gtest/gtest.h>

#include "wal/hashdex.h"

struct GoLabelSet {
  PromPP::Primitives::Go::Slice<char> data;
  PromPP::Primitives::Go::Slice<PromPP::Primitives::Go::LabelView> pairs;
};

struct GoTimeSeries {
  GoLabelSet label_set;
  uint64_t timestamp;
  double value;
};

template <>
struct BareBones::IsTriviallyReallocatable<GoTimeSeries> : std::true_type {};

template <class Labels>
void make_timeseries(Labels& labels, uint64_t timestamp, double value, PromPP::Primitives::TimeseriesSemiview& timeseries) {
  auto& label_set = timeseries.label_set();
  for (size_t i = 0; i < labels.size(); i += 2) {
    typename PromPP::Primitives::TimeseriesSemiview::label_set_type::label_type label;
    std::string_view name(labels[i]);
    std::get<0>(label) = name;
    std::string_view value(labels[i + 1]);
    std::get<1>(label) = value;
    label_set.add(label);
  }

  PromPP::Primitives::TimeseriesSemiview::samples_type::value_type sample;
  sample.timestamp() = timestamp;
  sample.value() = value;
  timeseries.samples().push_back(sample);
}

template <class Timeseries>
void timeseries_to_go_time_series(Timeseries& timeseries, GoTimeSeries& go_time_series) {
  size_t index = 0;

  for (auto& [name, value] : timeseries.label_set()) {
    PromPP::Primitives::Go::LabelView go_label_view;
    go_time_series.label_set.data.push_back(name.begin(), name.end());
    go_time_series.label_set.data.push_back(':');
    go_label_view.name = {static_cast<uint32_t>(index), static_cast<uint32_t>(name.size())};
    index += name.size() + 1;
    go_time_series.label_set.data.push_back(value.begin(), value.end());
    go_time_series.label_set.data.push_back(';');
    go_label_view.value = {static_cast<uint32_t>(index), static_cast<uint32_t>(value.size())};
    index += value.size() + 1;
    go_time_series.label_set.pairs.push_back(go_label_view);
  }
  go_time_series.timestamp = static_cast<uint64_t>(timeseries.samples().back().timestamp());
  go_time_series.value = timeseries.samples().back().value();
}

TEST(GoModelHashdexTest, HappyPath) {
  std::vector<std::string> label_set_a = {"name", "value", "cluster", "THECLUSTER", "__replica__", "rplc"};
  std::tuple<std::vector<std::string>, uint64_t, double> input_a = std::make_tuple(label_set_a, 25, 42);

  std::vector<std::string> label_set_b = {"abra", "kadabra"};
  std::tuple<std::vector<std::string>, uint64_t, double> input_b = std::make_tuple(label_set_b, 33, 66);

  std::vector<std::string> label_set_c = {"yet", "another", "awesome", "labelset"};
  std::tuple<std::vector<std::string>, uint64_t, double> input_c = std::make_tuple(label_set_c, 42, 25);

  std::vector<std::tuple<std::vector<std::string>, uint64_t, double>> input = {input_a, input_b, input_c};

  std::vector<PromPP::Primitives::TimeseriesSemiview> timeseries_vector;
  for (auto& tuple : input) {
    PromPP::Primitives::TimeseriesSemiview timeseries;
    std::vector<std::string>& labels = std::get<0>(tuple);
    uint64_t timestamp = std::get<1>(tuple);
    double value = std::get<2>(tuple);
    make_timeseries(labels, timestamp, value, timeseries);
    timeseries_vector.push_back(timeseries);
  }

  PromPP::Primitives::Go::Slice<GoTimeSeries> go_time_series_slice;

  for (auto& timeseries : timeseries_vector) {
    GoTimeSeries go_time_series;
    timeseries_to_go_time_series(timeseries, go_time_series);
    go_time_series_slice.push_back(go_time_series);
  }

  PromPP::Primitives::Go::SliceView<PromPP::Primitives::Go::TimeSeries>* go_time_series_slice_view =
      reinterpret_cast<PromPP::Primitives::Go::SliceView<PromPP::Primitives::Go::TimeSeries>*>(&go_time_series_slice);

  PromPP::WAL::GoModelHashdex hdx;
  hdx.presharding(*go_time_series_slice_view);

  EXPECT_EQ(hdx.cluster(), "THECLUSTER");
  EXPECT_EQ(hdx.replica(), "rplc");

  PromPP::Primitives::TimeseriesSemiview timeseries;
  auto ts_it = timeseries_vector.begin();
  for (auto hdx_it = hdx.begin(); hdx_it != hdx.end(); hdx_it++, ts_it++) {
    hdx_it->read(timeseries);
    EXPECT_TRUE(*ts_it == timeseries);
    timeseries.clear();
  }
}
