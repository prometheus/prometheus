#pragma once

#include "bare_bones/gorilla.h"
#include "bit_sequence.h"

namespace series_data::encoder {

class PROMPP_ATTRIBUTE_PACKED GorillaEncoder {
 public:
  PROMPP_ALWAYS_INLINE GorillaEncoder(int64_t timestamp, double value) {
    TimestampEncoder::encode(timestamp_state_, timestamp, stream_.stream);
    ValuesEncoder::encode_first(values_state_, value, stream_.stream);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_actual(double value) const noexcept {
    return std::bit_cast<uint64_t>(values_state_.last_v) == std::bit_cast<uint64_t>(value);
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t timestamp() const noexcept { return timestamp_state_.last_ts; }

  PROMPP_ALWAYS_INLINE uint8_t encode(int64_t timestamp, double value) {
    auto count = stream_.inc_count();

    if (count == 1) {
      [[unlikely]];
      TimestampEncoder::encode_delta(timestamp_state_, timestamp, stream_.stream);
      ValuesEncoder::encode(values_state_, value, stream_.stream);
    } else {
      TimestampEncoder::encode_delta_of_delta(timestamp_state_, timestamp, stream_.stream);
      ValuesEncoder::encode(values_state_, value, stream_.stream);
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

  BareBones::Encoding::Gorilla::TimestampEncoderState timestamp_state_;
  BareBones::Encoding::Gorilla::ValuesEncoderState values_state_;
  BitSequenceWithItemsCount stream_;
};

class GorillaDecoder {
 public:
  struct Sample {
    int64_t timestamp{};
    double value{};

    bool operator==(const Sample& other) const noexcept { return timestamp == other.timestamp && value::is_values_strictly_equals(value, other.value); }
  };

  using SampleList = BareBones::Vector<Sample>;

  explicit GorillaDecoder(BareBones::BitSequenceReader& reader) : reader_(reader) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE Sample decode() noexcept {
    decoder_.decode(reader_, reader_);
    return {.timestamp = decoder_.last_timestamp(), .value = decoder_.last_value()};
  }

  [[nodiscard]] static SampleList decode_all(BareBones::BitSequenceReader reader) noexcept {
    BareBones::Vector<Sample> samples;

    GorillaDecoder decoder(reader);
    while (!decoder.reader_.eof()) {
      samples.emplace_back(decoder.decode());
    }

    return samples;
  }

 private:
  BareBones::Encoding::Gorilla::StreamDecoder<BareBones::Encoding::Gorilla::ZigZagTimestampDecoder<>, BareBones::Encoding::Gorilla::ValuesDecoder> decoder_;
  BareBones::BitSequenceReader& reader_;
};

}  // namespace series_data::encoder

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::GorillaEncoder> : std::true_type {};
