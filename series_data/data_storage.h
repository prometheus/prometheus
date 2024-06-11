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
  encoder::timestamp::Encoder timestamp_encoder;

  BareBones::VectorWithHoles<encoder::value::DoubleConstantEncoder> double_constant_encoders;
  BareBones::VectorWithHoles<encoder::value::TwoDoubleConstantEncoder> two_double_constant_encoders;
  BareBones::VectorWithHoles<encoder::value::AscIntegerValuesGorillaEncoder> asc_integer_values_gorilla_encoders;
  BareBones::VectorWithHoles<encoder::value::ValuesGorillaEncoder> values_gorilla_encoders;
  BareBones::VectorWithHoles<encoder::GorillaEncoder> gorilla_encoders;

  size_t outdated_chunks_map_allocated_memory{};
  phmap::
      flat_hash_map<uint32_t, chunk::OutdatedChunk, std::hash<uint32_t>, std::equal_to<>, BareBones::Allocator<std::pair<const uint32_t, chunk::OutdatedChunk>>>
          outdated_chunks_{{}, {}, BareBones::Allocator<std::pair<const uint32_t, chunk::OutdatedChunk>>{outdated_chunks_map_allocated_memory}};

  BareBones::VectorWithHoles<encoder::RefCountableBitSequenceWithItemsCount> finalized_timestamp_streams;
  BareBones::VectorWithHoles<encoder::CompactBitSequence> finalized_data_streams;
  size_t finalized_chunks_map_allocated_memory{};
  phmap::flat_hash_map<uint32_t,
                       chunk::FinalizedChunkList,
                       std::hash<uint32_t>,
                       std::equal_to<>,
                       BareBones::Allocator<std::pair<const uint32_t, std::forward_list<chunk::DataChunk>>>>
      finalized_chunks{{}, {}, BareBones::Allocator<std::pair<const uint32_t, std::forward_list<chunk::DataChunk>>>{finalized_chunks_map_allocated_memory}};

  template <chunk::DataChunk::Type chunk_type>
  void erase_chunk(const chunk::DataChunk& chunk) {
    if (chunk.encoding_type != chunk::DataChunk::EncodingType::kGorilla) {
      erase_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id);
    }

    erase_encoder_data<chunk_type>(chunk);
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE const encoder::BitSequenceWithItemsCount& get_timestamp_stream(uint32_t stream_id) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return timestamp_encoder.get_stream(stream_id);
    } else {
      return finalized_timestamp_streams[stream_id].stream;
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE const encoder::CompactBitSequence& get_gorilla_encoder_stream(uint32_t stream_id) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return gorilla_encoders[stream_id].stream().stream;
    } else {
      return finalized_data_streams[stream_id];
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    size_t outdated_chunks_allocated_memory = 0;
    for (auto& [_, outdated_chunk] : outdated_chunks_) {
      outdated_chunks_allocated_memory += outdated_chunk.allocated_memory();
    }

    return open_chunks.allocated_memory() + double_constant_encoders.allocated_memory() + two_double_constant_encoders.allocated_memory() +
           asc_integer_values_gorilla_encoders.allocated_memory() + values_gorilla_encoders.allocated_memory() + gorilla_encoders.allocated_memory() +
           timestamp_encoder.allocated_memory() + finalized_timestamp_streams.allocated_memory() + finalized_data_streams.allocated_memory() +
           finalized_chunks_map_allocated_memory + outdated_chunks_map_allocated_memory + outdated_chunks_allocated_memory;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory(chunk::DataChunk::EncodingType type) const noexcept {
    using enum chunk::DataChunk::EncodingType;

    switch (type) {
      case kDoubleConstant: {
        return double_constant_encoders.allocated_memory();
      }

      case kTwoDoubleConstant: {
        return two_double_constant_encoders.allocated_memory();
      }

      case kAscIntegerValuesGorilla: {
        return asc_integer_values_gorilla_encoders.allocated_memory();
      }

      case kValuesGorilla: {
        return values_gorilla_encoders.allocated_memory();
      }

      case kGorilla: {
        return gorilla_encoders.allocated_memory();
      }

      case kUint32Constant:
      case kUnknown: {
        return 0;
      }

      default: {
        assert(type != kUnknown);
        return 0;
      }
    }
  }

 private:
  template <chunk::DataChunk::Type chunk_type>
  void erase_timestamp_stream(uint32_t stream_id) {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      timestamp_encoder.erase(stream_id);
    } else {
      if (--finalized_timestamp_streams[stream_id].reference_count == 0) {
        finalized_timestamp_streams.erase(stream_id);
      }
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  void erase_encoder_data(const chunk::DataChunk& chunk) {
    using enum chunk::DataChunk::EncodingType;

    switch (chunk.encoding_type) {
      case kDoubleConstant: {
        double_constant_encoders.erase(chunk.encoder.double_constant);
        break;
      }

      case kTwoDoubleConstant: {
        two_double_constant_encoders.erase(chunk.encoder.two_double_constant);
        break;
      }

      case kAscIntegerValuesGorilla: {
        if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
          asc_integer_values_gorilla_encoders.erase(chunk.encoder.asc_integer_values_gorilla);
        } else {
          finalized_data_streams.erase(chunk.encoder.asc_integer_values_gorilla);
        }
        break;
      }

      case kValuesGorilla: {
        if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
          values_gorilla_encoders.erase(chunk.encoder.values_gorilla);
        } else {
          finalized_data_streams.erase(chunk.encoder.values_gorilla);
        }
        break;
      }

      case kGorilla: {
        if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
          gorilla_encoders.erase(chunk.encoder.gorilla);
        } else {
          finalized_data_streams.erase(chunk.encoder.gorilla);
        }
        break;
      }

      case kUint32Constant:
      case kUnknown: {
        break;
      }
    }
  }
};

}  // namespace series_data