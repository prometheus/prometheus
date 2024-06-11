#pragma once

#include "bare_bones/preprocess.h"
#include "series_data/encoder/bit_sequence.h"
#include "series_data/encoder/sample.h"
#include "series_data/encoder/timestamp/encoder.h"

namespace series_data::decoder {

class DecodeIteratorSentinel {};

class DecodeIteratorTrait {
 public:
  using iterator_category = std::forward_iterator_tag;
  using value_type = encoder::Sample;
  using difference_type = ptrdiff_t;
  using pointer = encoder::Sample*;
  using reference = encoder::Sample&;

  explicit DecodeIteratorTrait(uint8_t count) : remaining_samples_{count} {}
  explicit DecodeIteratorTrait(double value, uint8_t count) : sample_{.value = value}, remaining_samples_{count} {}

  const encoder::Sample& operator*() const noexcept { return sample_; }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const noexcept { return remaining_samples_ == 0; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint8_t remaining_samples() const noexcept { return remaining_samples_; }

 protected:
  encoder::Sample sample_;
  uint8_t remaining_samples_{};
};

class SeparatedTimestampValueDecodeIteratorTrait : public DecodeIteratorTrait {
 public:
  explicit SeparatedTimestampValueDecodeIteratorTrait(const encoder::BitSequenceWithItemsCount& timestamp_stream)
      : DecodeIteratorTrait(timestamp_stream.count()), timestamp_decoder_(timestamp_stream.reader()) {
    if (remaining_samples_ > 0) {
      sample_.timestamp = timestamp_decoder_.decode();
    }
  }
  SeparatedTimestampValueDecodeIteratorTrait(const encoder::BitSequenceWithItemsCount& timestamp_stream, double value)
      : DecodeIteratorTrait(value, timestamp_stream.count()), timestamp_decoder_(timestamp_stream.reader()) {
    sample_.timestamp = timestamp_decoder_.decode();
  }

  PROMPP_ALWAYS_INLINE bool decode_timestamp() noexcept {
    if (--remaining_samples_ > 0) {
      sample_.timestamp = timestamp_decoder_.decode();
      return true;
    }

    return false;
  }

 protected:
  encoder::timestamp::TimestampDecoder timestamp_decoder_;
};

}  // namespace series_data::decoder