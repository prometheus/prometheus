#pragma once

#include "data_storage.h"

namespace series_data {

inline constexpr uint8_t kSamplesPerChunkDefault = 240;

template <uint8_t kSamplesPerChunk = kSamplesPerChunkDefault>
class Encoder {
 public:
  void encode(uint32_t ls_id, int64_t timestamp, double value) {
    auto& chunk = (storage_.open_chunks.size() > ls_id) ? storage_.open_chunks[ls_id] : storage_.open_chunks.emplace_back();
    if (chunk.encoding_type == chunk::DataChunk::EncodingType::kGorilla) {
      [[unlikely]];
      encode_gorilla(ls_id, timestamp, value, chunk);
    } else {
      encode_timestamp_and_value_separately(ls_id, timestamp, value, chunk);
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return storage_.allocated_memory(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory(chunk::DataChunk::EncodingType type) const noexcept { return storage_.allocated_memory(type); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const DataStorage& storage() const noexcept { return storage_; }

 private:
  DataStorage storage_;

  void encode_gorilla(uint32_t ls_id, int64_t timestamp, double value, chunk::DataChunk& chunk) {
    auto& encoder = storage_.gorilla_encoders_[chunk.encoder.gorilla];
    if (timestamp > encoder.timestamp()) {
      [[likely]];
      if (encoder.encode(timestamp, value) >= kSamplesPerChunk) {
        [[unlikely]];
        finalize_chunk(ls_id, chunk, encoder::timestamp::State::kInvalidId);
      }
    } else if (timestamp < encoder.timestamp() || !encoder.is_actual(value)) {
      encode_outdated_chunk(ls_id, timestamp, value);
    }
  }

  void encode_timestamp_and_value_separately(uint32_t ls_id, int64_t timestamp, double value, chunk::DataChunk& chunk) {
    if (chunk.timestamp_encoder_state_id != encoder::timestamp::State::kInvalidId) {
      [[likely]];

      auto& state = storage_.timestamp_encoder_.get_state(chunk.timestamp_encoder_state_id);
      if (timestamp > state.timestamp()) {
        [[likely]];

        if (auto finalized_stream_id = storage_.timestamp_encoder_.process_finalized(chunk.timestamp_encoder_state_id);
            finalized_stream_id != encoder::timestamp::State::kInvalidId) {
          [[unlikely]];
          finalize_chunk(ls_id, chunk, finalized_stream_id);
        } else if (state.stream_data.stream.count() >= kSamplesPerChunk) {
          [[unlikely]];
          finalize_timestamp_state_and_chunk(ls_id, chunk);
        }
      } else {
        if (timestamp == state.timestamp()) {
          if (is_actual_value(chunk, value)) {
            return;
          }

          // change encoder type if possible
        }

        encode_outdated_chunk(ls_id, timestamp, value);
        return;
      }
    }

    encode_value(ls_id, chunk, timestamp, value);
    if (chunk.encoding_type != chunk::DataChunk::EncodingType::kGorilla) {
      chunk.timestamp_encoder_state_id = storage_.timestamp_encoder_.encode(chunk.timestamp_encoder_state_id, timestamp);
    }
  }

  PROMPP_ALWAYS_INLINE void finalize_timestamp_state_and_chunk(uint32_t ls_id, chunk::DataChunk& chunk) {
    auto finalized_timestamp_stream_id = storage_.finalized_timestamp_streams_.size();
    storage_.timestamp_encoder_.finalize(chunk.timestamp_encoder_state_id, storage_.finalized_timestamp_streams_.emplace_back(), finalized_timestamp_stream_id);
    finalize_chunk(ls_id, chunk, finalized_timestamp_stream_id);
  }

  PROMPP_ALWAYS_INLINE void finalize_chunk(uint32_t ls_id, chunk::DataChunk& chunk, uint32_t finalized_timestamp_stream_id) {
    auto& finalized_chunk = emplace_finalized_chunk(ls_id, chunk, finalized_timestamp_stream_id);

    if (chunk.encoding_type == chunk::DataChunk::EncodingType::kAscIntegerValuesGorilla) {
      finalized_chunk.encoder.asc_integer_values_gorilla = storage_.finalized_data_streams_.size();
      storage_.finalized_data_streams_.emplace_back(storage_.asc_integer_values_gorilla_encoders_[chunk.encoder.asc_integer_values_gorilla].finalize_stream());
      storage_.asc_integer_values_gorilla_encoders_.erase(chunk.encoder.asc_integer_values_gorilla);
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kValuesGorilla) {
      finalized_chunk.encoder.values_gorilla = storage_.finalized_data_streams_.size();
      storage_.finalized_data_streams_.emplace_back(storage_.values_gorilla_encoders_[chunk.encoder.values_gorilla].finalize_stream());
      storage_.values_gorilla_encoders_.erase(chunk.encoder.values_gorilla);
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kGorilla) {
      finalized_chunk.encoder.gorilla = storage_.finalized_data_streams_.size();
      storage_.finalized_data_streams_.emplace_back(storage_.gorilla_encoders_[chunk.encoder.gorilla].finalize_stream());
      storage_.gorilla_encoders_.erase(chunk.encoder.gorilla);
    }

    chunk.reset();
  }

  PROMPP_ALWAYS_INLINE chunk::DataChunk& emplace_finalized_chunk(uint32_t ls_id, chunk::DataChunk& chunk, uint32_t finalized_timestamp_stream_id) {
    return storage_.finalized_chunks_.try_emplace(ls_id, storage_.finalized_chunks_map_allocated_memory_)
        .first->second.emplace_back(chunk, finalized_timestamp_stream_id);
  }

  PROMPP_ALWAYS_INLINE void encode_outdated_chunk(uint32_t ls_id, int64_t timestamp, double value) {
    if (auto it = storage_.outdated_chunks_.try_emplace(ls_id, timestamp, value); !it.second) {
      it.first->second.encode(timestamp, value);
    }
  }

  void encode_value(uint32_t ls_id, chunk::DataChunk& chunk, int64_t timestamp, double value) {
    if (chunk.encoding_type == chunk::DataChunk::EncodingType::kUnknown) {
      [[unlikely]];

      if (encoder::value::Uint32ConstantEncoder::can_be_encoded(value)) {
        chunk.encoding_type = chunk::DataChunk::EncodingType::kUint32Constant;
        new (&chunk.encoder) encoder::value::Uint32ConstantEncoder(value);
      } else {
        switch_to_double_constant_encoder(chunk, value);
      }
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kUint32Constant) {
      if (!chunk.encoder.uint32_constant.encode(value)) {
        switch_to_two_constant_encoder(chunk, chunk.encoder.uint32_constant.value(), value);
      }
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kDoubleConstant) {
      if (auto& encoder = storage_.double_constant_encoders_[chunk.encoder.double_constant]; !encoder.encode(value)) {
        auto encoder_id = chunk.encoder.double_constant;
        switch_to_two_constant_encoder(chunk, encoder.value(), value);
        storage_.double_constant_encoders_.erase(encoder_id);
      }
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kTwoDoubleConstant) {
      if (auto& encoder = storage_.two_double_constant_encoders_[chunk.encoder.two_double_constant]; !encoder.encode(value)) {
        auto encoder_id = chunk.encoder.two_double_constant;

        if (encoder::value::AscIntegerValuesGorillaEncoder::can_be_encoded(encoder.value1(), encoder.value1_count(), encoder.value2(), value)) {
          [[likely]];
          switch_to_asc_integer_values_gorilla(chunk, encoder, value);
        } else if (!storage_.timestamp_encoder_.is_unique_state(chunk.timestamp_encoder_state_id)) {
          switch_to_values_gorilla(chunk, encoder, value);
        } else {
          switch_to_gorilla(chunk, encoder, timestamp, value);
        }

        storage_.two_double_constant_encoders_.erase(encoder_id);
      }
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kAscIntegerValuesGorilla) {
      if (!storage_.asc_integer_values_gorilla_encoders_[chunk.encoder.asc_integer_values_gorilla].encode(value)) {
        auto finalized_timestamp_stream_id = storage_.finalized_timestamp_streams_.size();
        storage_.timestamp_encoder_.finalize_or_copy(chunk.timestamp_encoder_state_id, storage_.finalized_timestamp_streams_.emplace_back(),
                                                     finalized_timestamp_stream_id);
        finalize_chunk(ls_id, chunk, finalized_timestamp_stream_id);

        switch_to_double_constant_encoder(chunk, value);
      }
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kValuesGorilla) {
      storage_.values_gorilla_encoders_[chunk.encoder.values_gorilla].encode(value);
    }
  }

  [[nodiscard]] bool is_actual_value(const chunk::DataChunk& chunk, double value) const noexcept {
    if (chunk.encoding_type == chunk::DataChunk::EncodingType::kUint32Constant) {
      return chunk.encoder.uint32_constant.is_actual(value);
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kDoubleConstant) {
      return storage_.double_constant_encoders_[chunk.encoder.double_constant].is_actual(value);
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kTwoDoubleConstant) {
      return storage_.two_double_constant_encoders_[chunk.encoder.two_double_constant].is_actual(value);
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kAscIntegerValuesGorilla) {
      return storage_.asc_integer_values_gorilla_encoders_[chunk.encoder.asc_integer_values_gorilla].is_actual(value);
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kValuesGorilla) {
      return storage_.values_gorilla_encoders_[chunk.encoder.values_gorilla].is_actual(value);
    }

    return false;
  }

  PROMPP_ALWAYS_INLINE void switch_to_double_constant_encoder(chunk::DataChunk& chunk, double value) {
    chunk.encoding_type = chunk::DataChunk::EncodingType::kDoubleConstant;
    auto& encoder = storage_.double_constant_encoders_.emplace_back(value);
    chunk.encoder.double_constant = storage_.double_constant_encoders_.index_of(encoder);
  }

  PROMPP_ALWAYS_INLINE void switch_to_two_constant_encoder(chunk::DataChunk& chunk, double value1, double value2) {
    auto& encoder = storage_.two_double_constant_encoders_.emplace_back(
        value1, value2, static_cast<uint8_t>(storage_.timestamp_encoder_.get_stream(chunk.timestamp_encoder_state_id).count()));
    chunk.encoding_type = chunk::DataChunk::EncodingType::kTwoDoubleConstant;
    chunk.encoder.two_double_constant = storage_.two_double_constant_encoders_.index_of(encoder);
  }

  void switch_to_asc_integer_values_gorilla(chunk::DataChunk& data, const encoder::value::TwoDoubleConstantEncoder& constant_encoder, double value) {
    auto& encoder = storage_.asc_integer_values_gorilla_encoders_.emplace_back(constant_encoder.value1());
    auto value1_count = constant_encoder.value1_count();
    auto value2_count = static_cast<uint8_t>(storage_.timestamp_encoder_.get_stream(data.timestamp_encoder_state_id).count() - constant_encoder.value1_count());

    if (value1_count > 1) {
      encoder.encode_second(constant_encoder.value1());
      for (uint8_t i = 2; i < value1_count; ++i) {
        encoder.encode(constant_encoder.value1());
      }
    } else {
      encoder.encode_second(constant_encoder.value2());
      --value2_count;
    }

    for (uint8_t i = 0; i < value2_count; ++i) {
      encoder.encode(constant_encoder.value2());
    }

    encoder.encode(value);

    data.encoding_type = chunk::DataChunk::EncodingType::kAscIntegerValuesGorilla;
    data.encoder.asc_integer_values_gorilla = storage_.asc_integer_values_gorilla_encoders_.index_of(encoder);
  }

  void switch_to_values_gorilla(chunk::DataChunk& data, const encoder::value::TwoDoubleConstantEncoder& constant_encoder, double value) {
    auto& encoder = storage_.values_gorilla_encoders_.emplace_back(constant_encoder.value1(), constant_encoder.value1_count());

    auto value2_count = storage_.timestamp_encoder_.get_stream(data.timestamp_encoder_state_id).count() - constant_encoder.value1_count();
    encoder.encode(constant_encoder.value2(), value2_count);

    encoder.encode(value);

    data.encoding_type = chunk::DataChunk::EncodingType::kValuesGorilla;
    data.encoder.values_gorilla = storage_.values_gorilla_encoders_.index_of(encoder);
  }

  void switch_to_gorilla(chunk::DataChunk& chunk, const encoder::value::TwoDoubleConstantEncoder& constant_encoder, int64_t timestamp, double value) {
    auto& timestamp_stream = storage_.timestamp_encoder_.get_stream(chunk.timestamp_encoder_state_id);
    encoder::timestamp::TimestampDecoder timestamp_decoder(timestamp_stream.reader());
    uint8_t value2_count = timestamp_stream.count() - constant_encoder.value1_count();

    auto& encoder = storage_.gorilla_encoders_.emplace_back(timestamp_decoder.decode(), constant_encoder.value1());

    for (uint8_t i = 1; i < constant_encoder.value1_count(); ++i) {
      encoder.encode(timestamp_decoder.decode(), constant_encoder.value1());
    }

    for (uint8_t i = 0; i < value2_count; ++i) {
      encoder.encode(timestamp_decoder.decode(), constant_encoder.value2());
    }

    encoder.encode(timestamp, value);

    storage_.timestamp_encoder_.erase(chunk.timestamp_encoder_state_id);

    chunk.encoding_type = chunk::DataChunk::EncodingType::kGorilla;
    chunk.encoder.gorilla = storage_.gorilla_encoders_.index_of(encoder);
  }
};

}  // namespace series_data