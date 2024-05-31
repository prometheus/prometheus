#pragma once

#include <chrono>

#include "bare_bones/preprocess.h"
#include "series_data/encoder/gorilla.h"

namespace series_data::chunk {

#pragma pack(push, 1)
class OutdatedChunk {
 public:
  OutdatedChunk(int64_t timestamp, double value) : encoder_(timestamp, value), create_time_(Clock::now()) {}

  PROMPP_ALWAYS_INLINE uint8_t encode(int64_t timestamp, double value) { return encoder_.encode(timestamp, value); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint8_t count() const noexcept { return encoder_.stream().count(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const encoder::BitSequenceWithItemsCount& stream() const noexcept { return encoder_.stream(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return encoder_.allocated_memory(); }

 private:
  using Clock = std::chrono::system_clock;

  encoder::GorillaEncoder encoder_;
  Clock::time_point create_time_;
};
#pragma pack(pop)

}  // namespace series_data::chunk