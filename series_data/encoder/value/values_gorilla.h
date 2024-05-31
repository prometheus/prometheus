#pragma once

#include "bare_bones/gorilla.h"
#include "series_data/encoder/bit_sequence.h"

namespace series_data::encoder::value {

class PROMPP_ATTRIBUTE_PACKED ValuesGorillaEncoder {
 public:
  PROMPP_ALWAYS_INLINE explicit ValuesGorillaEncoder(double value, uint32_t count) { Encoder::encode_first(state_, value, count, stream_); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_actual(double value) const noexcept { return is_values_strictly_equals(state_.last_v, value); }

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

class ValuesGorillaDecoder {
 public:
  explicit ValuesGorillaDecoder(BareBones::BitSequenceReader reader) : reader_(reader) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE double decode() noexcept {
    if (first_) {
      [[unlikely]];
      Decoder::decode_first(state_, reader_);
      first_ = false;
    } else {
      Decoder::decode(state_, reader_);
    }

    return state_.last_v;
  }

  [[nodiscard]] static BareBones::Vector<double> decode_all(BareBones::BitSequenceReader reader) noexcept {
    BareBones::Vector<double> values;

    ValuesGorillaDecoder decoder(reader);
    while (!decoder.reader_.eof()) {
      values.emplace_back(decoder.decode());
    }

    return values;
  }

 private:
  using Decoder = BareBones::Encoding::Gorilla::ValuesDecoder;

  BareBones::BitSequenceReader reader_;
  BareBones::Encoding::Gorilla::ValuesDecoderState state_;
  bool first_{true};
};

}  // namespace series_data::encoder::value

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::value::ValuesGorillaEncoder> : std::true_type {};
