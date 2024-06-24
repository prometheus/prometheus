#pragma once

#include <ostream>

#include "series_data/chunk/serialized_chunk.h"
#include "series_data/data_storage.h"
#include "series_data/querier/query.h"

namespace series_data::serialization {

class Serializer {
 public:
  explicit Serializer(const DataStorage& storage) : storage_(storage) {}

  void serialize(const querier::QueriedChunkList& queried_chunks, std::ostream& stream) {
    auto data_offset = write_chunks_count(queried_chunks, stream);

    TimestampStreamsData timestamp_streams_data;
    {
      auto serialized_chunks = create_serialized_chunks(queried_chunks, timestamp_streams_data, data_offset);
      write_serialized_chunks(serialized_chunks, stream);
    }

    write_chunks_data(queried_chunks, timestamp_streams_data, stream);
  }

 private:
  struct TimestampStreamsData {
    using TimestampId = uint32_t;
    using Offset = uint32_t;

    static constexpr Offset kInvalidOffset = std::numeric_limits<Offset>::max();

    phmap::flat_hash_map<TimestampId, Offset> stream_offsets;
    phmap::flat_hash_map<TimestampId, Offset> finalized_stream_offsets;
  };

  using QueriedChunk = querier::QueriedChunk;
  using QueriedChunkList = querier::QueriedChunkList;
  using SerializedChunk = chunk::SerializedChunk;
  using SerializedChunkList = chunk::SerializedChunkList;

  const DataStorage& storage_;

  PROMPP_ALWAYS_INLINE static uint32_t write_chunks_count(const QueriedChunkList& queried_chunks, std::ostream& stream) {
    uint32_t count = queried_chunks.size();
    stream.write(reinterpret_cast<char*>(&count), sizeof(count));
    return sizeof(count);
  }

  PROMPP_ALWAYS_INLINE static void write_serialized_chunks(const SerializedChunkList& chunks, std::ostream& stream) noexcept {
    auto size_in_bytes = chunks.size() * sizeof(SerializedChunk);
    stream.write(reinterpret_cast<const char*>(chunks.data()), static_cast<std::streamsize>(size_in_bytes));
  }

  [[nodiscard]] SerializedChunkList create_serialized_chunks(const QueriedChunkList& queried_chunks,
                                                             TimestampStreamsData& timestamp_streams_data,
                                                             uint32_t& data_offset) const noexcept {
    SerializedChunkList serialized_chunks;
    uint32_t chunks_size = queried_chunks.size();
    serialized_chunks.reserve(chunks_size);
    data_offset += sizeof(SerializedChunk) * chunks_size;

    for (auto& queried_chunk : queried_chunks) {
      if (queried_chunk.is_open()) {
        fill_serialized_chunk<chunk::DataChunk::Type::kOpen>(queried_chunk, serialized_chunks.emplace_back(queried_chunk.ls_id), timestamp_streams_data,
                                                             data_offset);
      } else {
        fill_serialized_chunk<chunk::DataChunk::Type::kFinalized>(queried_chunk, serialized_chunks.emplace_back(queried_chunk.ls_id), timestamp_streams_data,
                                                                  data_offset);
      }
    }

    return serialized_chunks;
  }

  template <chunk::DataChunk::Type chunk_type>
  void fill_serialized_chunk(const QueriedChunk& queried_chunk,
                             SerializedChunk& serialized_chunk,
                             TimestampStreamsData& timestamp_streams_data,
                             uint32_t& data_offset) const noexcept {
    using enum chunk::DataChunk::EncodingType;

    auto& chunk = get_chunk<chunk_type>(queried_chunk);
    serialized_chunk.encoding_type = chunk.encoding_type;

    switch (chunk.encoding_type) {
      case kUint32Constant: {
        fill_timestamp_stream_offset<chunk_type>(timestamp_streams_data, chunk.timestamp_encoder_state_id, serialized_chunk, data_offset);
        serialized_chunk.values_offset = std::bit_cast<uint32_t>(chunk.encoder.uint32_constant);
        break;
      }

      case kDoubleConstant: {
        fill_timestamp_stream_offset<chunk_type>(timestamp_streams_data, chunk.timestamp_encoder_state_id, serialized_chunk, data_offset);
        serialized_chunk.values_offset = data_offset;
        data_offset += sizeof(encoder::value::DoubleConstantEncoder);
        break;
      }

      case kTwoDoubleConstant: {
        fill_timestamp_stream_offset<chunk_type>(timestamp_streams_data, chunk.timestamp_encoder_state_id, serialized_chunk, data_offset);
        serialized_chunk.values_offset = data_offset;
        data_offset += sizeof(encoder::value::TwoDoubleConstantEncoder);
        break;
      }

      case kAscIntegerValuesGorilla: {
        fill_timestamp_stream_offset<chunk_type>(timestamp_streams_data, chunk.timestamp_encoder_state_id, serialized_chunk, data_offset);

        serialized_chunk.values_offset = data_offset;
        data_offset += storage_.get_asc_integer_values_gorilla_stream<chunk_type>(chunk.encoder.asc_integer_values_gorilla).size_in_bytes();
        break;
      }

      case kValuesGorilla: {
        fill_timestamp_stream_offset<chunk_type>(timestamp_streams_data, chunk.timestamp_encoder_state_id, serialized_chunk, data_offset);

        serialized_chunk.values_offset = data_offset;
        data_offset += storage_.get_values_gorilla_stream<chunk_type>(chunk.encoder.values_gorilla).size_in_bytes();
        break;
      }

      case kGorilla: {
        serialized_chunk.values_offset = data_offset;
        data_offset += storage_.get_gorilla_encoder_stream<chunk_type>(chunk.encoder.gorilla).size_in_bytes();
        break;
      }

      case kUnknown: {
        assert(chunk.encoding_type != kUnknown);
      }
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] const chunk::DataChunk& get_chunk(const QueriedChunk& queried_chunk) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return storage_.open_chunks[queried_chunk.ls_id];
    } else {
      auto finalized_chunk_it = storage_.finalized_chunks.find(queried_chunk.ls_id)->second.begin();
      std::advance(finalized_chunk_it, queried_chunk.finalized_chunk_id);
      return *finalized_chunk_it;
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  void fill_timestamp_stream_offset(TimestampStreamsData& timestamp_streams_data,
                                    encoder::timestamp::State::Id timestamp_stream_id,
                                    SerializedChunk& serialized_chunk,
                                    uint32_t& data_offset) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      if (auto it = timestamp_streams_data.stream_offsets.find(timestamp_stream_id); it != timestamp_streams_data.stream_offsets.end()) {
        serialized_chunk.timestamps_offset = it->second;
        return;
      }

      timestamp_streams_data.stream_offsets.emplace(timestamp_stream_id, data_offset);
    } else {
      if (auto it = timestamp_streams_data.finalized_stream_offsets.find(timestamp_stream_id); it != timestamp_streams_data.finalized_stream_offsets.end()) {
        serialized_chunk.timestamps_offset = it->second;
        return;
      }

      timestamp_streams_data.finalized_stream_offsets.emplace(timestamp_stream_id, data_offset);
    }

    serialized_chunk.timestamps_offset = data_offset;
    data_offset += storage_.get_timestamp_stream<chunk_type>(timestamp_stream_id).stream.size_in_bytes();
  }

  void write_chunks_data(const QueriedChunkList& queried_chunks, TimestampStreamsData& timestamp_streams_data, std::ostream& stream) noexcept {
    for (auto& queried_chunk : queried_chunks) {
      if (queried_chunk.is_open()) {
        write_chunk_data<chunk::DataChunk::Type::kOpen>(get_chunk<chunk::DataChunk::Type::kOpen>(queried_chunk), timestamp_streams_data, stream);
      } else {
        write_chunk_data<chunk::DataChunk::Type::kFinalized>(get_chunk<chunk::DataChunk::Type::kFinalized>(queried_chunk), timestamp_streams_data, stream);
      }
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  void write_chunk_data(const chunk::DataChunk& chunk, TimestampStreamsData& timestamp_streams_data, std::ostream& stream) noexcept {
    using enum chunk::DataChunk::EncodingType;

    switch (chunk.encoding_type) {
      case kUint32Constant: {
        write_timestamp_stream<chunk_type>(timestamp_streams_data, chunk.timestamp_encoder_state_id, stream);
        break;
      }

      case kDoubleConstant: {
        write_timestamp_stream<chunk_type>(timestamp_streams_data, chunk.timestamp_encoder_state_id, stream);
        stream.write(reinterpret_cast<const char*>(&storage_.double_constant_encoders[chunk.encoder.double_constant]),
                     sizeof(encoder::value::DoubleConstantEncoder));
        break;
      }

      case kTwoDoubleConstant: {
        write_timestamp_stream<chunk_type>(timestamp_streams_data, chunk.timestamp_encoder_state_id, stream);
        stream.write(reinterpret_cast<const char*>(&storage_.two_double_constant_encoders[chunk.encoder.two_double_constant]),
                     sizeof(encoder::value::TwoDoubleConstantEncoder));
        break;
      }

      case kAscIntegerValuesGorilla: {
        write_timestamp_stream<chunk_type>(timestamp_streams_data, chunk.timestamp_encoder_state_id, stream);
        write_compact_bit_sequence(storage_.get_asc_integer_values_gorilla_stream<chunk_type>(chunk.encoder.asc_integer_values_gorilla), stream);
        break;
      }

      case kValuesGorilla: {
        write_timestamp_stream<chunk_type>(timestamp_streams_data, chunk.timestamp_encoder_state_id, stream);
        write_compact_bit_sequence(storage_.get_values_gorilla_stream<chunk_type>(chunk.encoder.values_gorilla), stream);
        break;
      }

      case kGorilla: {
        write_compact_bit_sequence(storage_.get_gorilla_encoder_stream<chunk_type>(chunk.encoder.gorilla), stream);
        break;
      }

      case kUnknown: {
        assert(chunk.encoding_type != kUnknown);
        break;
      }
    }
  }

  template <class CompactBitSequence>
  PROMPP_ALWAYS_INLINE void write_compact_bit_sequence(const CompactBitSequence& bit_sequence, std::ostream& stream) {
    stream.write(reinterpret_cast<const char*>(bit_sequence.raw_bytes()), bit_sequence.size_in_bytes());
  }

  template <chunk::DataChunk::Type chunk_type>
  PROMPP_ALWAYS_INLINE void write_timestamp_stream(TimestampStreamsData& timestamp_streams_data,
                                                   encoder::timestamp::State::Id stream_id,
                                                   std::ostream& stream) {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      if (auto it = timestamp_streams_data.stream_offsets.find(stream_id); it->second != TimestampStreamsData::kInvalidOffset) {
        write_compact_bit_sequence(storage_.get_timestamp_stream<chunk_type>(stream_id).stream, stream);
        it->second = TimestampStreamsData::kInvalidOffset;
      }
    } else {
      if (auto it = timestamp_streams_data.finalized_stream_offsets.find(stream_id); it->second != TimestampStreamsData::kInvalidOffset) {
        write_compact_bit_sequence(storage_.get_timestamp_stream<chunk_type>(stream_id).stream, stream);
        it->second = TimestampStreamsData::kInvalidOffset;
      }
    }
  }
};

}  // namespace series_data::serialization