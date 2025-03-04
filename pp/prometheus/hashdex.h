#pragma once

#include <string_view>

#include "bare_bones/preprocess.h"

namespace PromPP::Prometheus::hashdex {

struct Limits {
  uint32_t max_label_name_length;
  uint32_t max_label_value_length;
  uint32_t max_label_names_per_timeseries;
  size_t max_timeseries_count;
};

class Abstract {
 public:
  [[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view replica() const noexcept { return replica_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view cluster() const noexcept { return cluster_; }

 protected:
  std::string_view replica_;
  std::string_view cluster_;

  template <class LabelSet>
  void set_cluser_and_replica_values(const LabelSet& label_set) {
    for (const auto& [name, value] : label_set) {
      if (name == "cluster") {
        cluster_ = value;
      }
      if (name == "__replica__") {
        replica_ = value;
      }
    }
  }
};

template <class Hashdex>
concept HashdexInterface = requires(const Hashdex& const_hashdex) {
  { const_hashdex.size() } -> std::convertible_to<size_t>;

  { std::forward_iterator<decltype(const_hashdex.begin())> };
  { const_hashdex.begin() == const_hashdex.end() };

  { const_hashdex.metadata() };
  { const_hashdex.metrics() };
};

}  // namespace PromPP::Prometheus::hashdex