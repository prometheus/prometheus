#pragma once

#include <chrono>
#include <string_view>

#include "bare_bones/preprocess.h"
#include "primitives/primitives.h"
#include "prometheus/value.h"

namespace PromPP::WAL {

class HeartbeatMetricsInserter {
 public:
  struct Data {
    std::string_view agent_uuid;
    std::string_view hostname;
  };

  explicit HeartbeatMetricsInserter(const Data& data) : data_(data) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool need_insert_metrics() const noexcept { return !data_.hostname.empty(); }

  template <class TimeseriesInserter>
  void insert(std::chrono::system_clock::time_point now, std::chrono::nanoseconds sent_at, TimeseriesInserter&& insert_timeseries) {
    auto now_ms = std::chrono::duration_cast<std::chrono::milliseconds>(now.time_since_epoch());

    auto& timeseries = create_timeseries("okagent__timestamp", "heartbeat", now_ms, sent_at);
    timeseries.label_set().add(std::initializer_list<PromPP::Primitives::LabelView>{
        {"conf", "/usr/local/okagent/etc/config.yaml"},
        {"okmeter_plugin", "heartbeat"},
        {"okmeter_plugin_instance", "/usr/local/okagent/etc/config.yaml"},
    });
    insert_timeseries(timeseries);

    insert_timeseries(create_timeseries("okagent__heartbeat", "collector", now_ms, std::chrono::seconds(1)));
    insert_timeseries(create_timeseries("time__offset__collector", "collector", now_ms, (now_ms - sent_at)));
  }

 private:
  const Data data_;
  PromPP::Primitives::TimeseriesSemiview timeseries_;

  PROMPP_ALWAYS_INLINE PromPP::Primitives::TimeseriesSemiview& create_timeseries(std::string_view name,
                                                                               std::string_view job,
                                                                               std::chrono::milliseconds timestamp,
                                                                               std::chrono::nanoseconds value) {
    timeseries_.clear();

    timeseries_.label_set().add(std::initializer_list<PromPP::Primitives::LabelView>{
        {Prometheus::kMetricLabelName, name},
        {"instance", data_.hostname},
        {"agent_uuid", data_.agent_uuid},
        {"job", job},
    });
    timeseries_.samples().emplace_back(timestamp.count(), std::chrono::duration<double>(value).count());

    return timeseries_;
  }
};

}  // namespace PromPP::WAL