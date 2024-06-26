#pragma once

#include "bare_bones/gorilla.h"
#include "series_data/encoder/bit_sequence.h"

namespace series_data::encoder::value {

class PROMPP_ATTRIBUTE_PACKED ValuesGorillaEncoder {
 public:
  PROMPP_ALWAYS_INLINE explicit ValuesGorillaEncoder(double value, uint32_t count) { Encoder::encode_first(state_, value, count, stream_); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_actual(double value) const noexcept { return is_values_strictly_equals(state_.last_v, value); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE double last_value() const noexcept { return state_.last_v; }

  PROMPP_ALWAYS_INLINE void encode(double value) { Encoder::encode(state_, value, stream_); }

  PROMPP_ALWAYS_INLINE void encode(double value, uint32_t count) { Encoder::encode(state_, value, count, stream_); }

  bool operator==(const ValuesGorillaEncoder& other) const noexcept { return stream_ == other.stream_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return stream_.allocated_memory(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const CompactBitSequence& stream() const noexcept { return stream_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE CompactBitSequence finalize_stream() noexcept {
    auto stream = std::move(stream_);
    stream.shrink_to_fit();
    return stream;
  }

 private:
  using Encoder = BareBones::Encoding::Gorilla::ValuesEncoder;

  BareBones::Encoding::Gorilla::ValuesEncoderState state_;
  BareBones::CompactBitSequence<kAllocationSizesTable> stream_;
};

}  // namespace series_data::encoder::value

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::value::ValuesGorillaEncoder> : std::true_type {};
