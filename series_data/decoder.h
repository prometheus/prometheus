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
  template <chunk::DataChunk::Type chunk_type, class Callback>
  static void decode_chunk(const DataStorage& storage, const chunk::DataChunk& chunk, Callback&& callback) {
    using enum chunk::DataChunk::EncodingType;
    using decoder::DecodeIteratorSentinel;

    switch (chunk.encoding_type) {
      case kUint32Constant: {
        std::ranges::for_each(create_decode_iterator<kUint32Constant, chunk_type>(storage, chunk), DecodeIteratorSentinel{}, std::forward<Callback>(callback));
        break;
      }

      case kDoubleConstant: {
        std::ranges::for_each(create_decode_iterator<kDoubleConstant, chunk_type>(storage, chunk), DecodeIteratorSentinel{}, std::forward<Callback>(callback));
        break;
      }

      case kTwoDoubleConstant: {
        std::ranges::for_each(create_decode_iterator<kTwoDoubleConstant, chunk_type>(storage, chunk), DecodeIteratorSentinel{},
                              std::forward<Callback>(callback));
        break;
      }

      case kAscIntegerValuesGorilla: {
        std::ranges::for_each(create_decode_iterator<kAscIntegerValuesGorilla, chunk_type>(storage, chunk), DecodeIteratorSentinel{},
                              std::forward<Callback>(callback));
        break;
      }

      case kValuesGorilla: {
        std::ranges::for_each(create_decode_iterator<kValuesGorilla, chunk_type>(storage, chunk), DecodeIteratorSentinel{}, std::forward<Callback>(callback));
        break;
      }

      case kGorilla: {
        decode_gorilla_chunk<Callback>(storage.get_gorilla_encoder_stream<chunk_type>(chunk.encoder.gorilla), std::forward<Callback>(callback));
        break;
      }

      case kUnknown: {
        assert(chunk.encoding_type != kUnknown);
        break;
      }
    }
  }

  template <class Callback>
  static void decode_chunk(const DataStorage& storage, const chunk::DataChunk& chunk, chunk::DataChunk::Type chunk_type, Callback&& callback) {
    if (chunk_type == chunk::DataChunk::Type::kOpen) {
      decode_chunk<chunk::DataChunk::Type::kOpen>(storage, chunk, std::forward<Callback>(callback));
    } else {
      decode_chunk<chunk::DataChunk::Type::kFinalized>(storage, chunk, std::forward<Callback>(callback));
    }
  }

  static uint8_t get_samples_count(const DataStorage& storage, const chunk::DataChunk& chunk, chunk::DataChunk::Type chunk_type) noexcept {
    using enum chunk::DataChunk::Type;

    if (chunk.encoding_type == chunk::DataChunk::EncodingType::kGorilla) {
      [[unlikely]];
      return encoder::BitSequenceWithItemsCount::count(chunk_type == kOpen ? storage.get_values_gorilla_stream<kOpen>(chunk.encoder.gorilla)
                                                                           : storage.get_values_gorilla_stream<kFinalized>(chunk.encoder.gorilla));
    } else {
      return (chunk_type == kOpen ? storage.get_timestamp_stream<kOpen>(chunk.timestamp_encoder_state_id)
                                  : storage.get_timestamp_stream<kFinalized>(chunk.timestamp_encoder_state_id))
          .count();
    }
  }

  template <class Callback>
  PROMPP_ALWAYS_INLINE static void decode_gorilla_chunk(const encoder::CompactBitSequence& stream, Callback&& callback) {
    std::ranges::for_each(decoder::GorillaDecodeIterator(stream), decoder::DecodeIteratorSentinel{}, std::forward<Callback>(callback));
  }

  template <chunk::DataChunk::Type chunk_type>
  static encoder::SampleList decode_chunk(const DataStorage& storage, const chunk::DataChunk& chunk) {
    encoder::SampleList result;
    decode_chunk<chunk_type>(storage, chunk, [&result](const encoder::Sample& sample) PROMPP_LAMBDA_INLINE { result.emplace_back(sample); });
    return result;
  }

  PROMPP_ALWAYS_INLINE static encoder::SampleList decode_gorilla_chunk(const encoder::CompactBitSequence& stream) {
    encoder::SampleList result;
    result.reserve(encoder::BitSequenceWithItemsCount::count(stream));
    decode_gorilla_chunk(stream, [&result](const encoder::Sample& sample) PROMPP_LAMBDA_INLINE { result.emplace_back(sample); });
    return result;
  }

  static BareBones::Vector<encoder::SampleList> decode_chunks(const DataStorage& storage, const chunk::FinalizedChunkList& chunks) {
    BareBones::Vector<encoder::SampleList> result;
    for (auto& chunk : chunks) {
      auto& samples = result.emplace_back();
      decode_chunk<chunk::DataChunk::Type::kFinalized>(storage, chunk,
                                                       [&samples](const encoder::Sample& sample) PROMPP_LAMBDA_INLINE { samples.emplace_back(sample); });
    }
    return result;
  }

  static BareBones::Vector<encoder::SampleList> decode_chunks(const DataStorage& storage,
                                                              const chunk::FinalizedChunkList& finalized_chunks,
                                                              const chunk::DataChunk& open_chunk) {
    auto result = decode_chunks(storage, finalized_chunks);
    auto& open_chunk_samples = result.emplace_back();
    decode_chunk<chunk::DataChunk::Type::kOpen>(
        storage, open_chunk, [&open_chunk_samples](const encoder::Sample& sample) PROMPP_LAMBDA_INLINE { open_chunk_samples.emplace_back(sample); });
    return result;
  }

  template <chunk::DataChunk::EncodingType encoding_type, chunk::DataChunk::Type chunk_type>
  static auto create_decode_iterator(const DataStorage& storage, const chunk::DataChunk& chunk) {
    using enum chunk::DataChunk::EncodingType;
    using enum chunk::DataChunk::Type;

    if constexpr (encoding_type == kUint32Constant) {
      return decoder::ConstantDecodeIterator(storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id), chunk.encoder.uint32_constant.value());
    } else if constexpr (encoding_type == kDoubleConstant) {
      return decoder::ConstantDecodeIterator(storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
                                             storage.double_constant_encoders[chunk.encoder.double_constant].value());
    } else if constexpr (encoding_type == kTwoDoubleConstant) {
      return decoder::TwoDoubleConstantDecodeIterator(storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
                                                      storage.two_double_constant_encoders[chunk.encoder.two_double_constant]);
    } else if constexpr (encoding_type == kAscIntegerValuesGorilla) {
      return decoder::AscIntegerValuesGorillaDecodeIterator(
          storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
          chunk_type == kOpen ? storage.asc_integer_values_gorilla_encoders[chunk.encoder.asc_integer_values_gorilla].stream().reader()
                              : storage.finalized_data_streams[chunk.encoder.asc_integer_values_gorilla].reader());
    } else if constexpr (encoding_type == kValuesGorilla) {
      return decoder::ValuesGorillaDecodeIterator(storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
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

  [[nodiscard]] PROMPP_ALWAYS_INLINE static int64_t get_open_chunk_last_timestamp(const DataStorage& storage, const chunk::DataChunk& chunk) noexcept {
    if (chunk.encoding_type == chunk::DataChunk::EncodingType::kGorilla) {
      [[unlikely]];
      return storage.gorilla_encoders[chunk.encoder.gorilla].timestamp();
    }

    return storage.timestamp_encoder.get_state(chunk.timestamp_encoder_state_id).timestamp();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static int64_t get_finalized_chunk_last_timestamp(const DataStorage& storage,
                                                                                       uint32_t ls_id,
                                                                                       chunk::FinalizedChunkList::ChunksList::const_iterator chunk_it,
                                                                                       chunk::FinalizedChunkList::ChunksList::const_iterator end_it) noexcept {
    if (auto next_chunk_it = std::next(chunk_it); next_chunk_it != end_it) {
      return get_chunk_first_timestamp<chunk::DataChunk::Type::kFinalized>(storage, *next_chunk_it);
    } else {
      return get_chunk_first_timestamp<chunk::DataChunk::Type::kOpen>(storage, storage.open_chunks[ls_id]);
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static double get_open_chunk_last_value(const DataStorage& storage, const chunk::DataChunk& chunk) noexcept {
    using enum chunk::DataChunk::EncodingType;

    switch (chunk.encoding_type) {
      case kUint32Constant: {
        return chunk.encoder.uint32_constant.value();
      }

      case kDoubleConstant: {
        return storage.double_constant_encoders[chunk.encoder.double_constant].value();
      }

      case kTwoDoubleConstant: {
        return storage.two_double_constant_encoders[chunk.encoder.two_double_constant].value2();
      }

      case kAscIntegerValuesGorilla: {
        return storage.asc_integer_values_gorilla_encoders[chunk.encoder.asc_integer_values_gorilla].last_value();
      }

      case kValuesGorilla: {
        return storage.values_gorilla_encoders[chunk.encoder.values_gorilla].last_value();
      }

      case kGorilla: {
        return storage.gorilla_encoders[chunk.encoder.gorilla].last_value();
      }

      default: {
        assert(chunk.encoding_type != kUint32Constant);
        return 0.0;
      }
    }
  }

 private:
  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] static BareBones::BitSequenceReader get_stream_reader(const DataStorage& storage, const chunk::DataChunk& chunk) {
    if (chunk.encoding_type != chunk::DataChunk::EncodingType::kGorilla) {
      [[likely]];
      return storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id).reader();
    }

    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return storage.gorilla_encoders[chunk.encoder.gorilla].stream().reader();
    } else {
      return encoder::BitSequenceWithItemsCount::reader(storage.finalized_data_streams[chunk.encoder.gorilla]);
    }
  }
};

}  // namespace series_data