#pragma once

#include "data_storage.h"

namespace series_data {

inline constexpr uint8_t kSamplesPerChunkDefault = 240;

template <class EncoderType>
concept EncoderInterface = requires(EncoderType& encoder, EncoderType& const_encoder, chunk::DataChunk& chunk, const chunk::DataChunk& const_chunk) {
  { encoder.storage() } -> std::same_as<DataStorage&>;
  { encoder.encode(uint32_t(), int64_t(), double(), chunk) };
};

struct FakeEncoder {
  FakeEncoder() = delete;
  FakeEncoder(const FakeEncoder&) = delete;
  FakeEncoder(FakeEncoder&&) noexcept = delete;

  static DataStorage& storage() noexcept {
    static DataStorage s;
    return s;
  }
  void encode(uint32_t, int64_t, double, chunk::DataChunk&) {}
};

static_assert(EncoderInterface<FakeEncoder>);

template <class OutdatedSampleEncoder>
concept OutdatedSampleEncoderInterface = requires(OutdatedSampleEncoder& outdated_sample_encoder, FakeEncoder& encoder) {
  { outdated_sample_encoder.encode(encoder, uint32_t{}, int64_t{}, double{}) };
};

}  // namespace series_data