#pragma once

#include "primitives/primitives.h"
#include "prometheus/metric.h"

namespace PromPP::WAL::hashdex {

struct Metric {
  Primitives::TimeseriesSemiview timeseries{};
  uint64_t hash{};

  bool operator==(const Metric&) const noexcept = default;
};

#pragma pack(push, 1)

struct Metadata {
  std::string_view metric_name{};
  std::string_view text{};
  Prometheus::MetadataType type{};

  bool operator==(const Metadata&) const noexcept = default;
};

#pragma pack(pop)

}  // namespace PromPP::WAL::hashdex