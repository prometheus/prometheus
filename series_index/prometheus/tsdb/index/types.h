#pragma once

#include <cstdint>
#include <vector>

#include "parallel_hashmap/phmap.h"
#include "primitives/primitives.h"
#include "prometheus/tsdb/index/types.h"

namespace series_index::prometheus::tsdb::index {

struct SymbolLssId {
  static constexpr uint32_t kNoId = std::numeric_limits<uint32_t>::max();

  uint32_t name_id;
  uint32_t value_id;

  explicit constexpr SymbolLssId(uint32_t _name_id = kNoId, uint32_t _value_id = kNoId) : name_id(_name_id), value_id(_value_id) {}

  explicit operator uint64_t() const noexcept { return *reinterpret_cast<const uint64_t*>(this); }

  bool operator==(const SymbolLssId&) const noexcept = default;

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const noexcept { return name_id == kNoId && value_id == kNoId; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_name() const noexcept { return value_id == kNoId; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_duplicated() const noexcept { return name_id == kNoId && value_id == 0; }

  PROMPP_ALWAYS_INLINE void mark_as_duplicated() noexcept {
    name_id = kNoId;
    value_id = 0;
  }
};

using SymbolReferencesMap = phmap::flat_hash_map<SymbolLssId, PromPP::Prometheus::tsdb::index::SymbolReference, std::hash<SymbolLssId>>;
using SeriesReferencesMap =
    phmap::flat_hash_map<PromPP::Primitives::LabelSetID, PromPP::Prometheus::tsdb::index::SeriesReference, std::hash<PromPP::Primitives::LabelSetID>>;

struct ChunkMetadata {
  PromPP::Primitives::Timestamp min_timestamp{};
  PromPP::Primitives::Timestamp max_timestamp{};
  uint64_t reference{};
};

}  // namespace series_index::prometheus::tsdb::index

template <>
struct std::hash<series_index::prometheus::tsdb::index::SymbolLssId> {
  PROMPP_ALWAYS_INLINE size_t operator()(const series_index::prometheus::tsdb::index::SymbolLssId& id) const noexcept { return id.operator uint64_t(); }
};