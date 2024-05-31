#pragma once

#include "bare_bones/preprocess.h"
#include "chunk/data_chunk.h"
#include "chunk/finalized_chunk.h"
#include "chunk/outdated_chunk.h"
#include "encoder/gorilla.h"
#include "encoder/value/asc_integer_values_gorilla.h"
#include "encoder/value/double_constant.h"
#include "encoder/value/two_double_constant.h"
#include "encoder/value/uint32_constant.h"
#include "encoder/value/values_gorilla.h"
#include "series_data/encoder/timestamp/encoder.h"

namespace series_data {

struct DataStorage {
  BareBones::Vector<chunk::DataChunk> open_chunks;
  encoder::timestamp::Encoder timestamp_encoder_;

  BareBones::VectorWithHoles<encoder::value::DoubleConstantEncoder> double_constant_encoders_;
  BareBones::VectorWithHoles<encoder::value::TwoDoubleConstantEncoder> two_double_constant_encoders_;
  BareBones::VectorWithHoles<encoder::value::AscIntegerValuesGorillaEncoder> asc_integer_values_gorilla_encoders_;
  BareBones::VectorWithHoles<encoder::value::ValuesGorillaEncoder> values_gorilla_encoders_;
  BareBones::VectorWithHoles<encoder::GorillaEncoder> gorilla_encoders_;

  size_t outdated_chunks_map_allocated_memory_{};
  phmap::
      flat_hash_map<uint32_t, chunk::OutdatedChunk, std::hash<uint32_t>, std::equal_to<>, BareBones::Allocator<std::pair<const uint32_t, chunk::OutdatedChunk>>>
          outdated_chunks_{{}, {}, BareBones::Allocator<std::pair<const uint32_t, chunk::OutdatedChunk>>{outdated_chunks_map_allocated_memory_}};

  BareBones::Vector<encoder::BitSequenceWithItemsCount> finalized_timestamp_streams_;
  BareBones::Vector<encoder::CompactBitSequence> finalized_data_streams_;
  size_t finalized_chunks_map_allocated_memory_{};
  phmap::flat_hash_map<uint32_t,
                       chunk::FinalizedChunkList,
                       std::hash<uint32_t>,
                       std::equal_to<>,
                       BareBones::Allocator<std::pair<const uint32_t, std::forward_list<chunk::DataChunk>>>>
      finalized_chunks_{{}, {}, BareBones::Allocator<std::pair<const uint32_t, std::forward_list<chunk::DataChunk>>>{finalized_chunks_map_allocated_memory_}};

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    size_t outdated_chunks_allocated_memory = 0;
    for (auto& [_, outdated_chunk] : outdated_chunks_) {
      outdated_chunks_allocated_memory += outdated_chunk.allocated_memory();
    }

    return open_chunks.allocated_memory() + double_constant_encoders_.allocated_memory() + two_double_constant_encoders_.allocated_memory() +
           asc_integer_values_gorilla_encoders_.allocated_memory() + values_gorilla_encoders_.allocated_memory() + gorilla_encoders_.allocated_memory() +
           timestamp_encoder_.allocated_memory() + finalized_timestamp_streams_.allocated_memory() + finalized_data_streams_.allocated_memory() +
           finalized_chunks_map_allocated_memory_ + outdated_chunks_map_allocated_memory_ + outdated_chunks_allocated_memory;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory(chunk::DataChunk::EncodingType type) const noexcept {
    switch (type) {
      case chunk::DataChunk::EncodingType::kDoubleConstant: {
        return double_constant_encoders_.allocated_memory();
      }

      case chunk::DataChunk::EncodingType::kTwoDoubleConstant: {
        return two_double_constant_encoders_.allocated_memory();
      }

      case chunk::DataChunk::EncodingType::kAscIntegerValuesGorilla: {
        return asc_integer_values_gorilla_encoders_.allocated_memory();
      }

      case chunk::DataChunk::EncodingType::kValuesGorilla: {
        return values_gorilla_encoders_.allocated_memory();
      }

      case chunk::DataChunk::EncodingType::kGorilla: {
        return gorilla_encoders_.allocated_memory();
      }

      default: {
        return 0;
      }
    }
  }
};

}  // namespace series_data