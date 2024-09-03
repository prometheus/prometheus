#pragma once

#include <cstdint>

namespace PromPP::Prometheus::tsdb::index {

using SymbolReference = uint32_t;
using SeriesReference = uint32_t;

inline constexpr uint32_t kMagic = 0xBAAAD700;
inline constexpr uint8_t kFormatVersion = 2;
inline constexpr uint32_t kSeriesAlignment = 16;

}  // namespace PromPP::Prometheus::tsdb::index
