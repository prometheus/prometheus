#pragma once

#include "bare_bones/concepts.h"
#include "series_data/chunk/serialized_chunk.h"
#include "series_data/data_storage.h"
#include "series_data/encoder/bit_sequence.h"
#include "series_data/querier/query.h"

namespace series_data::serialization {

class Serializer {
 public:
  explicit Serializer(const DataStorage& storage) : storage_(storage) {}

  template <class Stream>
  void serialize(const querier::QueriedChunkList& queried_chunks, Stream& stream) {
    serialize_impl(queried_chunks, stream);
  }

  template <class Stream>
  void serialize(Stream& stream) {
    serialize_impl(storage_.chunks(), stream);
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

  template <class ChunkList, class Stream>
  void serialize_impl(const ChunkList& chunks, Stream& stream) {
    static constinit std::array<char, encoder::CompactBitSequence::reserved_size_in_bytes()> kReservedBytesForReader{};

    TimestampStreamsData timestamp_streams_data;
    {
      uint32_t data_size = sizeof(uint32_t);
      uint32_t chunk_count = get_chunk_count(chunks);
      auto serialized_chunks = create_serialized_chunks(chunks, chunk_count, timestamp_streams_data, data_size);

      if constexpr (BareBones::concepts::has_reserve<Stream>) {
        stream.reserve(data_size + kReservedBytesForReader.size());
      }

      write_chunk_count(chunk_count, stream);
      write_serialized_chunks(serialized_chunks, stream);
    }

    write_chunks_data(chunks, timestamp_streams_data, stream);
    stream.write(kReservedBytesForReader.data(), kReservedBytesForReader.size());
  }

  template <class ChunkList>
  PROMPP_ALWAYS_INLINE static uint32_t get_chunk_count(const ChunkList& chunks) noexcept {
    if constexpr (std::is_same_v<ChunkList, DataStorage::Chunks>) {
      return chunks.non_empty_chunk_count();
    } else {
      return chunks.size();
    }
  }

  PROMPP_ALWAYS_INLINE static void write_chunk_count(uint32_t chunk_count, std::ostream& stream) {
    stream.write(reinterpret_cast<char*>(&chunk_count), sizeof(chunk_count));
  }

  PROMPP_ALWAYS_INLINE static void write_serialized_chunks(const SerializedChunkList& chunks, std::ostream& stream) noexcept {
    const auto size_in_bytes = chunks.size() * sizeof(SerializedChunk);
    stream.write(reinterpret_cast<const char*>(chunks.data()), static_cast<std::streamsize>(size_in_bytes));
  }

  template <class ChunkList>
  [[nodiscard]] SerializedChunkList create_serialized_chunks(const ChunkList& chunks,
                                                             uint32_t chunk_count,
                                                             TimestampStreamsData& timestamp_streams_data,
                                                             uint32_t& data_size) const noexcept {
    SerializedChunkList serialized_chunks;
    serialized_chunks.reserve(chunk_count);
    data_size += sizeof(SerializedChunk) * chunk_count;

    for (auto& chunk_data : chunks) {
      using enum chunk::DataChunk::Type;

      if (chunk_data.is_open()) [[likely]] {
        if (const auto& chunk = get_chunk<kOpen>(chunk_data); !chunk.is_empty()) [[likely]] {
          fill_serialized_chunk<kOpen>(chunk, serialized_chunks.emplace_back(chunk_data.series_id()), timestamp_streams_data, data_size);
        }
      } else {
        fill_serialized_chunk<kFinalized>(get_chunk<kFinalized>(chunk_data), serialized_chunks.emplace_back(chunk_data.series_id()), timestamp_streams_data,
                                          data_size);
      }
    }

    return serialized_chunks;
  }

  template <chunk::DataChunk::Type chunk_type>
  void fill_serialized_chunk(const chunk::DataChunk& chunk,
                             SerializedChunk& serialized_chunk,
                             TimestampStreamsData& timestamp_streams_data,
                             uint32_t& data_size) const noexcept {
    using enum chunk::DataChunk::EncodingType;

    serialized_chunk.encoding_type = chunk.encoding_type;

    switch (chunk.encoding_type) {
      case kUint32Constant: {
        fill_timestamp_stream_offset<chunk_type>(timestamp_streams_data, chunk.timestamp_encoder_state_id, serialized_chunk, data_size);
        serialized_chunk.values_offset = std::bit_cast<uint32_t>(chunk.encoder.uint32_constant);
        break;
      }

      case kDoubleConstant: {
        fill_timestamp_stream_offset<chunk_type>(timestamp_streams_data, chunk.timestamp_encoder_state_id, serialized_chunk, data_size);
        serialized_chunk.values_offset = data_size;
        data_size += sizeof(encoder::value::DoubleConstantEncoder);
        break;
      }

      case kTwoDoubleConstant: {
        fill_timestamp_stream_offset<chunk_type>(timestamp_streams_data, chunk.timestamp_encoder_state_id, serialized_chunk, data_size);
        serialized_chunk.values_offset = data_size;
        data_size += sizeof(encoder::value::TwoDoubleConstantEncoder);
        break;
      }

      case kAscIntegerValuesGorilla: {
        fill_timestamp_stream_offset<chunk_type>(timestamp_streams_data, chunk.timestamp_encoder_state_id, serialized_chunk, data_size);

        serialized_chunk.values_offset = data_size;
        data_size += storage_.get_asc_integer_values_gorilla_stream<chunk_type>(chunk.encoder.asc_integer_values_gorilla).size_in_bytes();
        break;
      }

      case kValuesGorilla: {
        fill_timestamp_stream_offset<chunk_type>(timestamp_streams_data, chunk.timestamp_encoder_state_id, serialized_chunk, data_size);

        serialized_chunk.values_offset = data_size;
        data_size += storage_.get_values_gorilla_stream<chunk_type>(chunk.encoder.values_gorilla).size_in_bytes();
        break;
      }

      case kGorilla: {
        serialized_chunk.values_offset = data_size;
        data_size += storage_.get_gorilla_encoder_stream<chunk_type>(chunk.encoder.gorilla).size_in_bytes();
        break;
      }

      default: {
        assert(chunk.encoding_type != kUnknown);
      }
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] const chunk::DataChunk& get_chunk(const QueriedChunk& queried_chunk) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return storage_.open_chunks[queried_chunk.series_id()];
    } else {
      auto finalized_chunk_it = storage_.finalized_chunks.find(queried_chunk.series_id())->second.begin();
      std::advance(finalized_chunk_it, queried_chunk.finalized_chunk_id);
      return *finalized_chunk_it;
    }
  }

  template <chunk::DataChunk::Type>
  [[nodiscard]] static const chunk::DataChunk& get_chunk(const DataStorage::SeriesChunkIterator::Data& chunk) noexcept {
    return chunk.chunk();
  }

  template <chunk::DataChunk::Type chunk_type>
  void fill_timestamp_stream_offset(TimestampStreamsData& timestamp_streams_data,
                                    encoder::timestamp::State::Id timestamp_stream_id,
                                    SerializedChunk& serialized_chunk,
                                    uint32_t& data_size) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      if (const auto it = timestamp_streams_data.stream_offsets.find(timestamp_stream_id); it != timestamp_streams_data.stream_offsets.end()) {
        serialized_chunk.timestamps_offset = it->second;
        return;
      }

      timestamp_streams_data.stream_offsets.emplace(timestamp_stream_id, data_size);
    } else {
      if (const auto it = timestamp_streams_data.finalized_stream_offsets.find(timestamp_stream_id);
          it != timestamp_streams_data.finalized_stream_offsets.end()) {
        serialized_chunk.timestamps_offset = it->second;
        return;
      }

      timestamp_streams_data.finalized_stream_offsets.emplace(timestamp_stream_id, data_size);
    }

    serialized_chunk.timestamps_offset = data_size;
    data_size += storage_.get_timestamp_stream<chunk_type>(timestamp_stream_id).stream.size_in_bytes();
  }

  template <class ChunkList>
  void write_chunks_data(const ChunkList& chunks, TimestampStreamsData& timestamp_streams_data, std::ostream& stream) noexcept {
    using enum chunk::DataChunk::Type;

    for (auto& chunk_data : chunks) {
      if (chunk_data.is_open()) [[likely]] {
        if (const auto& chunk = get_chunk<kOpen>(chunk_data); !chunk.is_empty()) [[likely]] {
          write_chunk_data<kOpen>(chunk, timestamp_streams_data, stream);
        }
      } else {
        write_chunk_data<kFinalized>(get_chunk<kFinalized>(chunk_data), timestamp_streams_data, stream);
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
  PROMPP_ALWAYS_INLINE static void write_compact_bit_sequence(const CompactBitSequence& bit_sequence, std::ostream& stream) {
    stream.write(reinterpret_cast<const char*>(bit_sequence.raw_bytes()), bit_sequence.size_in_bytes());
  }

  template <chunk::DataChunk::Type chunk_type>
  PROMPP_ALWAYS_INLINE void write_timestamp_stream(TimestampStreamsData& timestamp_streams_data,
                                                   encoder::timestamp::State::Id stream_id,
                                                   std::ostream& stream) {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      if (const auto it = timestamp_streams_data.stream_offsets.find(stream_id); it->second != TimestampStreamsData::kInvalidOffset) {
        write_compact_bit_sequence(storage_.get_timestamp_stream<chunk_type>(stream_id).stream, stream);
        it->second = TimestampStreamsData::kInvalidOffset;
      }
    } else {
      if (const auto it = timestamp_streams_data.finalized_stream_offsets.find(stream_id); it->second != TimestampStreamsData::kInvalidOffset) {
        write_compact_bit_sequence(storage_.get_timestamp_stream<chunk_type>(stream_id).stream, stream);
        it->second = TimestampStreamsData::kInvalidOffset;
      }
    }
  }
};

}  // namespace series_data::serialization
