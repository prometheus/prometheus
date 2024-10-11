#pragma once

#define PROTOZERO_USE_VIEW std::string_view

#include "bare_bones/vector.h"
#include "primitives/primitives.h"
#include "third_party/protozero/pbf_reader.hpp"

namespace series_data::querier {

template<typename Vector = BareBones::Vector<PromPP::Primitives::LabelSetID>>
struct Query {
  int64_t start_timestamp_ms{};
  int64_t end_timestamp_ms{};
  Vector label_set_ids;

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_valid() const noexcept { return !label_set_ids.empty() && end_timestamp_ms >= start_timestamp_ms; }

  bool operator==(const Query&) const noexcept = default;

  [[nodiscard]] static bool read_from_protobuf(std::string_view protobuf, Query& query) noexcept {
    enum Tag {
      kStartTimestampMs = 1,
      kEndTimestampMs = 2,
      kLabelSetIds = 3,
    };

    try {
      protozero::pbf_reader reader(protobuf);

      while (reader.next()) {
        switch (reader.tag()) {
          case kStartTimestampMs: {
            query.start_timestamp_ms = reader.get_int64();
            break;
          }

          case kEndTimestampMs: {
            query.end_timestamp_ms = reader.get_int64();
            break;
          }

          case kLabelSetIds: {
            const auto range = reader.get_packed_uint32();
            query.label_set_ids.reserve(range.size());
            for (auto ls_id : range) {
              query.label_set_ids.emplace_back(ls_id);
            }
            break;
          }

          default: {
            reader.skip();
            break;
          }
        }
      }
    } catch (...) {
      return false;
    }

    return query.is_valid();
  }
};

struct PROMPP_ATTRIBUTE_PACKED QueriedChunk {
  static constexpr uint32_t kOpenChunkId = std::numeric_limits<uint32_t>::max();

  const PromPP::Primitives::LabelSetID ls_id;
  const uint32_t finalized_chunk_id;

  explicit QueriedChunk(PromPP::Primitives::LabelSetID _ls_id) : ls_id(_ls_id), finalized_chunk_id(kOpenChunkId) {}
  QueriedChunk(PromPP::Primitives::LabelSetID _ls_id, uint32_t _finalized_chunk_id) : ls_id(_ls_id), finalized_chunk_id(_finalized_chunk_id) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_open() const noexcept { return finalized_chunk_id == kOpenChunkId; }

  bool operator==(const QueriedChunk&) const noexcept = default;
};

using QueriedChunkList = BareBones::Vector<QueriedChunk>;

}  // namespace series_data::querier

template <>
struct BareBones::IsTriviallyReallocatable<series_data::querier::QueriedChunk> : std::true_type {};
