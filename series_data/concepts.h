#pragma once

#include "chunk/data_chunk.h"

namespace series_data {

template <class OutdatedSampleEncoder>
concept OutdatedSampleEncoderInterface = requires(OutdatedSampleEncoder& encoder) {
  { encoder.encode(uint32_t{}, int64_t{}, double{}) };
};

template <class EncoderType>
concept EncoderInterface = requires(EncoderType& encoder, EncoderType& const_encoder, chunk::DataChunk& chunk, const chunk::DataChunk& const_chunk) {
  { encoder.encode(uint32_t(), int64_t(), double(), chunk) };
  { encoder.erase_finalized(uint32_t(), const_chunk) };
  { encoder.finalize(uint32_t(), chunk) };
  { encoder.replace(uint32_t(), const_chunk) };
};

}  // namespace series_data