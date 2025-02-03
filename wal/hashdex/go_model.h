#pragma once

#include "bare_bones/exception.h"
#include "primitives/go_model.h"
#include "primitives/label_set.h"
#include "prometheus/hashdex.h"

namespace PromPP::WAL::hashdex {

class GoModel : public Prometheus::hashdex::Abstract {
 public:
  PROMPP_ALWAYS_INLINE GoModel() = default;
  explicit PROMPP_ALWAYS_INLINE GoModel(const Prometheus::hashdex::Limits& limits) noexcept : limits_(limits) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept { return metrics_.size(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const Prometheus::hashdex::Limits& limits() const noexcept { return limits_; }

  PROMPP_ALWAYS_INLINE void presharding(const Primitives::Go::SliceView<Primitives::Go::TimeSeries>& go_timeseries) {
    if (limits_.max_timeseries_count && std::size(go_timeseries) > limits_.max_timeseries_count) [[unlikely]] {
      throw BareBones::Exception(0x1806e61dde4a3d6f, "Timeseries limit exceeded");
    }

    metrics_.reserve(std::size(metrics_) + std::size(go_timeseries));

    Primitives::LabelViewSet label_set;
    Primitives::Go::LabelSetLimits limits = {
        .max_label_name_length = limits_.max_label_name_length,
        .max_label_value_length = limits_.max_label_value_length,
        .max_label_count = limits_.max_label_names_per_timeseries,
    };

    for (auto& timeseries : go_timeseries) {
      Primitives::Go::read_label_set(timeseries.label_set, label_set, limits);

      if (metrics_.empty()) [[unlikely]] {
        set_cluser_and_replica_values(label_set);
      }

      metrics_.emplace_back(hash_value(label_set), timeseries);

      label_set.clear();
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept { return std::begin(metrics_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE auto end() const noexcept { return std::end(metrics_); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const auto& metrics() const noexcept { return metrics_; }
  [[nodiscard]] static PROMPP_ALWAYS_INLINE auto metadata() noexcept {
    struct Stub {};
    return Stub{};
  }

 private:
  class Item {
    size_t hash_;
    const Primitives::Go::TimeSeries& go_time_series_;

   public:
    PROMPP_ALWAYS_INLINE Item(size_t hash, const Primitives::Go::TimeSeries& go_time_series) : hash_(hash), go_time_series_(go_time_series) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t hash() const { return hash_; }

    template <class Timeseries>
    PROMPP_ALWAYS_INLINE void read(Timeseries& timeseries) const {
      Primitives::Go::read_timeseries(go_time_series_, timeseries);
    }
  };

  BareBones::Vector<Item> metrics_;
  const Prometheus::hashdex::Limits limits_{};
};

static_assert(Prometheus::hashdex::HashdexInterface<GoModel>);

}  // namespace PromPP::WAL::hashdex