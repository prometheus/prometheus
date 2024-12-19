#pragma once

#include <array>
#include <cstdint>

#include "bare_bones/preprocess.h"

namespace PromPP::Prometheus {

enum class MetadataType : uint8_t {
  kHelp = 0,
  kType,
  kUnit,
};

enum class MetricType : uint8_t {
  kUnknown = 0,
  kCounter,
  kGauge,
  kHistogram,
  kGaugeHistogram,
  kSummary,
  kInfo,
  kStateSet,
};

namespace internal {

using std::operator""sv;

inline constinit std::array kMetricTypeNames = {
    "UNKNOWN"sv, "COUNTER"sv, "GAUGE"sv, "HISTOGRAM"sv, "GAUGEHISTOGRAM"sv, "SUMMARY"sv, "INFO"sv, "STATESET"sv,
};

}  // namespace internal

[[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view MetricTypeToString(MetricType type) noexcept {
  return internal::kMetricTypeNames[static_cast<uint8_t>(type)];
}

[[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view MetricTypeToString(uint32_t type) noexcept {
  if (type < internal::kMetricTypeNames.size()) {
    return MetricTypeToString(static_cast<MetricType>(type));
  }

  return internal::kMetricTypeNames[static_cast<uint8_t>(MetricType::kUnknown)];
}

}  // namespace PromPP::Prometheus