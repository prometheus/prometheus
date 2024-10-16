#pragma once

#include <bit>
#include <string_view>

namespace PromPP::Prometheus {

constexpr auto kNormalNan = std::bit_cast<double>(0x7ff0000000000001ULL);
constexpr auto kStaleNan = std::bit_cast<double>(0x7ff0000000000002ULL);

constexpr bool is_stale_nan(double value) noexcept {
  return std::bit_cast<uint64_t>(value) == std::bit_cast<uint64_t>(kStaleNan);
}

constexpr std::string_view kMetricLabelName = "__name__";

}  // namespace PromPP::Prometheus