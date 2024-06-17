#pragma once

#include "chunk/data_chunk.h"

namespace series_data {

inline constexpr uint8_t kSamplesPerChunkDefault = 240;

template <class EncoderType>
concept EncoderInterface = requires(EncoderType& encoder, EncoderType& const_encoder, chunk::DataChunk& chunk, const chunk::DataChunk& const_chunk) {
  { encoder.encode(uint32_t(), int64_t(), double(), chunk) };
  { encoder.erase_finalized(uint32_t(), const_chunk) };
  { encoder.finalize(uint32_t(), chunk) };
  { encoder.replace(uint32_t(), const_chunk) };
};

struct FakeEncoder {
  FakeEncoder() = delete;
  FakeEncoder(const FakeEncoder&) = delete;
  FakeEncoder(FakeEncoder&&) noexcept = delete;

  void encode(uint32_t, int64_t, double, chunk::DataChunk&) {}
  void erase_finalized(uint32_t, const chunk::DataChunk&) {}
  void finalize(uint32_t, chunk::DataChunk&) {}
  void replace(uint32_t, const chunk::DataChunk&) {}
};

static_assert(EncoderInterface<FakeEncoder>);

template <class OutdatedSampleEncoder>
concept OutdatedSampleEncoderInterface = requires(OutdatedSampleEncoder& outdated_sample_encoder, FakeEncoder& encoder) {
  { outdated_sample_encoder.encode(encoder, uint32_t{}, int64_t{}, double{}) };
};

}  // namespace series_data