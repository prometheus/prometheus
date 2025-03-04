#pragma once

#include "bare_bones/gorilla.h"
#include "bit_sequence.h"
#include "sample.h"

namespace series_data::encoder {

class PROMPP_ATTRIBUTE_PACKED GorillaEncoder {
 public:
  PROMPP_ALWAYS_INLINE GorillaEncoder(int64_t timestamp, double value) {
    timestamp_encoder_.encode(timestamp, stream_.stream);
    values_encoder_.encode_first(value, stream_.stream);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_actual(double value) const noexcept {
    return std::bit_cast<uint64_t>(values_encoder_.value()) == std::bit_cast<uint64_t>(value);
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE double last_value() const noexcept { return values_encoder_.value(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t timestamp() const noexcept { return timestamp_encoder_.timestamp(); }

  PROMPP_ALWAYS_INLINE uint8_t encode(int64_t timestamp, double value) {
    const auto count = stream_.inc_count();

    if (count == 1) [[unlikely]] {
      timestamp_encoder_.encode_delta(timestamp, stream_.stream);
      values_encoder_.encode(value, stream_.stream);
    } else {
      timestamp_encoder_.encode_delta_of_delta(timestamp, stream_.stream);
      values_encoder_.encode(value, stream_.stream);
    }

    return count + 1;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE CompactBitSequence finalize_stream() noexcept {
    auto stream = std::move(stream_.stream);
    stream.shrink_to_fit();
    return stream;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return stream_.allocated_memory(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const BitSequenceWithItemsCount& stream() const noexcept { return stream_; }

 private:
  using TimestampEncoder = BareBones::Encoding::Gorilla::ZigZagTimestampEncoder<>;
  using ValuesEncoder = BareBones::Encoding::Gorilla::ValuesEncoder;

  TimestampEncoder timestamp_encoder_;
  ValuesEncoder values_encoder_;
  BitSequenceWithItemsCount stream_;
};

}  // namespace series_data::encoder

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::GorillaEncoder> : std::true_type {};
