#pragma once

#include "data_storage.h"
#include "decoder/asc_integer_values_gorilla.h"
#include "decoder/constant.h"
#include "decoder/gorilla.h"
#include "decoder/two_double_constant.h"
#include "decoder/values_gorilla.h"

namespace series_data {

class Decoder {
 public:
  template <chunk::DataChunk::Type chunk_type>
  static void decode_chunk(const DataStorage& storage, const chunk::DataChunk& chunk, encoder::SampleList& result) {
    using enum chunk::DataChunk::EncodingType;
    using decoder::DecodeIteratorSentinel;

    switch (chunk.encoding_type) {
      case kUint32Constant: {
        auto& stream = storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id);
        result.reserve(result.size() + stream.count());
        std::ranges::copy(create_decode_iterator<kUint32Constant, chunk_type>(storage, chunk), DecodeIteratorSentinel{}, std::back_insert_iterator(result));
        break;
      }

      case kDoubleConstant: {
        auto& stream = storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id);
        result.reserve(result.size() + stream.count());
        std::ranges::copy(create_decode_iterator<kDoubleConstant, chunk_type>(storage, chunk), DecodeIteratorSentinel{}, std::back_insert_iterator(result));
        break;
      }

      case kTwoDoubleConstant: {
        auto& stream = storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id);
        result.reserve(result.size() + stream.count());
        std::ranges::copy(create_decode_iterator<kTwoDoubleConstant, chunk_type>(storage, chunk), DecodeIteratorSentinel{}, std::back_insert_iterator(result));
        break;
      }

      case kAscIntegerValuesGorilla: {
        auto& stream = storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id);
        result.reserve(result.size() + stream.count());
        std::ranges::copy(create_decode_iterator<kAscIntegerValuesGorilla, chunk_type>(storage, chunk), DecodeIteratorSentinel{},
                          std::back_insert_iterator(result));
        break;
      }

      case kValuesGorilla: {
        auto& stream = storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id);
        result.reserve(result.size() + stream.count());
        std::ranges::copy(create_decode_iterator<kValuesGorilla, chunk_type>(storage, chunk), DecodeIteratorSentinel{}, std::back_insert_iterator(result));
        break;
      }

      case kGorilla: {
        decode_gorilla_chunk(storage.get_gorilla_encoder_stream<chunk_type>(chunk.encoder.gorilla), result);
        break;
      }

      case kUnknown: {
        assert(chunk.encoding_type != kUnknown);
        break;
      }
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  static encoder::SampleList decode_chunk(const DataStorage& storage, const chunk::DataChunk& chunk) {
    encoder::SampleList result;
    decode_chunk<chunk_type>(storage, chunk, result);
    return result;
  }

  PROMPP_ALWAYS_INLINE static void decode_gorilla_chunk(const encoder::CompactBitSequence& stream, encoder::SampleList& result) {
    auto iterator = decoder::GorillaDecodeIterator(stream);
    result.reserve(result.size() + iterator.remaining_samples());
    std::ranges::copy(iterator, decoder::DecodeIteratorSentinel{}, std::back_insert_iterator(result));
  }

  PROMPP_ALWAYS_INLINE static encoder::SampleList decode_gorilla_chunk(const encoder::CompactBitSequence& stream) {
    encoder::SampleList result;
    decode_gorilla_chunk(stream, result);
    return result;
  }

  static BareBones::Vector<encoder::SampleList> decode_chunks(const DataStorage& storage, const chunk::FinalizedChunkList& chunks) {
    BareBones::Vector<encoder::SampleList> result;
    for (auto& chunk : chunks) {
      decode_chunk<chunk::DataChunk::Type::kFinalized>(storage, chunk, result.emplace_back());
    }
    return result;
  }

  static BareBones::Vector<encoder::SampleList> decode_chunks(const DataStorage& storage,
                                                              const chunk::FinalizedChunkList& finalized_chunks,
                                                              const chunk::DataChunk& open_chunk) {
    BareBones::Vector<encoder::SampleList> result;
    for (auto& finalized_chunk : finalized_chunks) {
      decode_chunk<chunk::DataChunk::Type::kFinalized>(storage, finalized_chunk, result.emplace_back());
    }
    decode_chunk<chunk::DataChunk::Type::kOpen>(storage, open_chunk, result.emplace_back());
    return result;
  }

  template <chunk::DataChunk::EncodingType encoding_type, chunk::DataChunk::Type chunk_type>
  static auto create_decode_iterator(const DataStorage& storage, const chunk::DataChunk& chunk) {
    using enum chunk::DataChunk::EncodingType;
    using enum chunk::DataChunk::Type;

    if constexpr (encoding_type == kUint32Constant) {
      return decoder::ConstantDecodeIterator(storage.template get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
                                             chunk.encoder.uint32_constant.value());
    } else if constexpr (encoding_type == kDoubleConstant) {
      return decoder::ConstantDecodeIterator(storage.template get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
                                             storage.double_constant_encoders[chunk.encoder.double_constant].value());
    } else if constexpr (encoding_type == kTwoDoubleConstant) {
      return decoder::TwoDoubleConstantDecodeIterator(storage.template get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
                                                      storage.two_double_constant_encoders[chunk.encoder.two_double_constant]);
    } else if constexpr (encoding_type == kAscIntegerValuesGorilla) {
      return decoder::AscIntegerValuesGorillaDecodeIterator(
          storage.template get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
          chunk_type == kOpen ? storage.asc_integer_values_gorilla_encoders[chunk.encoder.asc_integer_values_gorilla].stream().reader()
                              : storage.finalized_data_streams[chunk.encoder.asc_integer_values_gorilla].reader());
    } else if constexpr (encoding_type == kValuesGorilla) {
      return decoder::ValuesGorillaDecodeIterator(storage.template get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
                                                  chunk_type == kOpen ? storage.values_gorilla_encoders[chunk.encoder.values_gorilla].stream().reader()
                                                                      : storage.finalized_data_streams[chunk.encoder.values_gorilla].reader());
    } else if constexpr (encoding_type == kGorilla) {
      return decoder::GorillaDecodeIterator(storage.get_gorilla_encoder_stream<chunk_type>(chunk.encoder.gorilla));
    } else {
      static_assert(encoding_type == kUnknown);
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE static int64_t get_chunk_first_timestamp(const DataStorage& storage, const chunk::DataChunk& chunk) noexcept {
    return encoder::timestamp::TimestampDecoder::decode_first(get_stream_reader<chunk_type>(storage, chunk));
  }

 private:
  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] static BareBones::BitSequenceReader get_stream_reader(const DataStorage& storage, const chunk::DataChunk& chunk) {
    if (chunk.encoding_type != chunk::DataChunk::EncodingType::kGorilla) {
      [[likely]];
      return storage.template get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id).reader();
    } else {
      if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
        return storage.gorilla_encoders[chunk.encoder.gorilla].stream().reader();
      } else {
        return encoder::BitSequenceWithItemsCount::reader(storage.finalized_data_streams[chunk.encoder.gorilla]);
      }
    }
  }
};

}  // namespace series_data