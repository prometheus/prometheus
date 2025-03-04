#pragma once

#include "chunk_finalizer.h"
#include "concepts.h"
#include "data_storage.h"
#include "decoder.h"

namespace series_data {

template <OutdatedSampleEncoderInterface OutdatedSampleEncoder, uint8_t kSamplesPerChunk = kSamplesPerChunkDefault>
class Encoder {
 public:
  Encoder(DataStorage& storage, OutdatedSampleEncoder& outdated_sample_encoder) : storage_(storage), outdated_sample_encoder_(outdated_sample_encoder) {}

  DataStorage& storage() noexcept { return storage_; }

  PROMPP_ALWAYS_INLINE void encode(uint32_t ls_id, int64_t timestamp, double value) {
    ++storage_.samples_count;

    if (storage_.open_chunks.size() <= ls_id) [[unlikely]] {
      storage_.open_chunks.resize(ls_id + 1);
    }

    encode(ls_id, timestamp, value, storage_.open_chunks[ls_id]);
  }

  PROMPP_ALWAYS_INLINE void encode(uint32_t ls_id, int64_t timestamp, double value, chunk::DataChunk& chunk) {
    if (chunk.encoding_type == chunk::DataChunk::EncodingType::kGorilla) [[unlikely]] {
      encode_gorilla(ls_id, timestamp, value, chunk);
    } else {
      encode_timestamp_and_value_separately(ls_id, timestamp, value, chunk);
    }
  }

 private:
  DataStorage& storage_;
  OutdatedSampleEncoder& outdated_sample_encoder_;

  void encode_gorilla(uint32_t ls_id, int64_t timestamp, double value, chunk::DataChunk& chunk) {
    if (auto& encoder = storage_.gorilla_encoders[chunk.encoder.gorilla]; timestamp > encoder.timestamp()) [[likely]] {
      if (encoder.stream().count() >= kSamplesPerChunk) [[unlikely]] {
        ChunkFinalizer::finalize(storage_, ls_id, chunk);
        encode_timestamp_and_value_separately(ls_id, timestamp, value, chunk);
      } else {
        encoder.encode(timestamp, value);
      }
    } else if (timestamp < encoder.timestamp()) {
      outdated_sample_encoder_.encode(*this, ls_id, timestamp, value);
    }
  }

  void encode_timestamp_and_value_separately(uint32_t ls_id, int64_t timestamp, double value, chunk::DataChunk& chunk) {
    if (chunk.timestamp_encoder_state_id != encoder::timestamp::State::kInvalidId) [[likely]] {
      if (const auto& state = storage_.timestamp_encoder.get_state(chunk.timestamp_encoder_state_id); timestamp > state.timestamp()) [[likely]] {
        if (!ChunkFinalizer::finalize_if_timestamp_finalized(storage_, ls_id, chunk)) [[likely]] {
          if (state.stream_data.stream.count() >= kSamplesPerChunk) [[unlikely]] {
            ChunkFinalizer::finalize(storage_, ls_id, chunk);
          }
        }
      } else {
        if (timestamp < state.timestamp()) {
          outdated_sample_encoder_.encode(*this, ls_id, timestamp, value);
        }
        return;
      }
    }

    encode_value(ls_id, chunk, timestamp, value);

    if (chunk.encoding_type != chunk::DataChunk::EncodingType::kGorilla) {
      chunk.timestamp_encoder_state_id = storage_.timestamp_encoder.encode(chunk.timestamp_encoder_state_id, timestamp);
    }
  }

  void encode_value(uint32_t ls_id, chunk::DataChunk& chunk, int64_t timestamp, double value) const {
    if (chunk.encoding_type == chunk::DataChunk::EncodingType::kUnknown) [[unlikely]] {
      if (encoder::value::Uint32ConstantEncoder::can_be_encoded(value)) {
        chunk.encoding_type = chunk::DataChunk::EncodingType::kUint32Constant;
        std::construct_at(&chunk.encoder.uint32_constant, encoder::value::Uint32ConstantEncoder(value));
      } else if (encoder::value::Float32ConstantEncoder::can_be_encoded(value)) [[unlikely]] {
        chunk.encoding_type = chunk::DataChunk::EncodingType::kFloat32Constant;
        std::construct_at(&chunk.encoder.float32_constant, encoder::value::Float32ConstantEncoder(value));
      } else {
        switch_to_double_constant_encoder(chunk, value);
      }
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kUint32Constant) {
      if (!chunk.encoder.uint32_constant.encode(value)) {
        switch_to_two_constant_encoder(chunk, chunk.encoder.uint32_constant.value(), value);
      }
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kFloat32Constant) {
      if (!chunk.encoder.float32_constant.encode(value)) {
        switch_to_two_constant_encoder(chunk, chunk.encoder.float32_constant.value(), value);
      }
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kDoubleConstant) {
      if (const auto& encoder = storage_.double_constant_encoders[chunk.encoder.double_constant]; !encoder.encode(value)) {
        const auto encoder_id = chunk.encoder.double_constant;
        switch_to_two_constant_encoder(chunk, encoder.value(), value);
        storage_.double_constant_encoders.erase(encoder_id);
      }
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kTwoDoubleConstant) {
      if (const auto& encoder = storage_.two_double_constant_encoders[chunk.encoder.two_double_constant]; !encoder.encode(value)) {
        const auto encoder_id = chunk.encoder.two_double_constant;

        if (encoder::value::AscIntegerValuesGorillaEncoder::can_be_encoded(encoder.value1(), encoder.value1_count(), encoder.value2(), value)) [[likely]] {
          switch_to_asc_integer_values_gorilla(chunk, encoder, value);
        } else if (!storage_.timestamp_encoder.is_unique_state(chunk.timestamp_encoder_state_id)) {
          switch_to_values_gorilla(chunk, encoder, value);
        } else {
          switch_to_gorilla(chunk, encoder, timestamp, value);
        }

        storage_.two_double_constant_encoders.erase(encoder_id);
      }
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kAscIntegerValuesGorilla) {
      if (!storage_.asc_integer_values_gorilla_encoders[chunk.encoder.asc_integer_values_gorilla].encode(value)) {
        ChunkFinalizer::finalize_timestamp_and_chunk_separately<ChunkFinalizer::FinalizeTimestampStateMode::kFinalizeOrCopy>(storage_, ls_id, chunk);

        switch_to_double_constant_encoder(chunk, value);
      }
    } else if (chunk.encoding_type == chunk::DataChunk::EncodingType::kValuesGorilla) {
      storage_.values_gorilla_encoders[chunk.encoder.values_gorilla].encode(value);
    }
  }

  PROMPP_ALWAYS_INLINE void switch_to_double_constant_encoder(chunk::DataChunk& chunk, double value) const {
    chunk.encoding_type = chunk::DataChunk::EncodingType::kDoubleConstant;
    const auto& encoder = storage_.double_constant_encoders.emplace_back(value);
    chunk.encoder.double_constant = storage_.double_constant_encoders.index_of(encoder);
  }

  PROMPP_ALWAYS_INLINE void switch_to_two_constant_encoder(chunk::DataChunk& chunk, double value1, double value2) const {
    const auto& encoder =
        storage_.two_double_constant_encoders.emplace_back(value1, value2, storage_.timestamp_encoder.get_stream(chunk.timestamp_encoder_state_id).count());
    chunk.encoding_type = chunk::DataChunk::EncodingType::kTwoDoubleConstant;
    chunk.encoder.two_double_constant = storage_.two_double_constant_encoders.index_of(encoder);
  }

  void switch_to_asc_integer_values_gorilla(chunk::DataChunk& data, const encoder::value::TwoDoubleConstantEncoder& constant_encoder, double value) const {
    auto& encoder = storage_.asc_integer_values_gorilla_encoders.emplace_back(constant_encoder.value1());
    const auto value1_count = constant_encoder.value1_count();
    auto value2_count = static_cast<uint8_t>(storage_.timestamp_encoder.get_stream(data.timestamp_encoder_state_id).count() - constant_encoder.value1_count());

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
    data.encoder.asc_integer_values_gorilla = storage_.asc_integer_values_gorilla_encoders.index_of(encoder);
  }

  void switch_to_values_gorilla(chunk::DataChunk& data, const encoder::value::TwoDoubleConstantEncoder& constant_encoder, double value) const {
    auto& encoder = storage_.values_gorilla_encoders.emplace_back(constant_encoder.value1(), constant_encoder.value1_count());

    const auto value2_count = storage_.timestamp_encoder.get_stream(data.timestamp_encoder_state_id).count() - constant_encoder.value1_count();
    encoder.encode(constant_encoder.value2(), value2_count);

    encoder.encode(value);

    data.encoding_type = chunk::DataChunk::EncodingType::kValuesGorilla;
    data.encoder.values_gorilla = storage_.values_gorilla_encoders.index_of(encoder);
  }

  void switch_to_gorilla(chunk::DataChunk& chunk, const encoder::value::TwoDoubleConstantEncoder& constant_encoder, int64_t timestamp, double value) const {
    auto& timestamp_stream = storage_.timestamp_encoder.get_stream(chunk.timestamp_encoder_state_id);
    encoder::timestamp::TimestampDecoder timestamp_decoder(timestamp_stream.reader());
    const uint8_t value2_count = timestamp_stream.count() - constant_encoder.value1_count();

    auto& encoder = storage_.gorilla_encoders.emplace_back(timestamp_decoder.decode(), constant_encoder.value1());

    for (uint8_t i = 1; i < constant_encoder.value1_count(); ++i) {
      encoder.encode(timestamp_decoder.decode(), constant_encoder.value1());
    }

    for (uint8_t i = 0; i < value2_count; ++i) {
      encoder.encode(timestamp_decoder.decode(), constant_encoder.value2());
    }

    encoder.encode(timestamp, value);

    storage_.timestamp_encoder.erase(chunk.timestamp_encoder_state_id);

    chunk.encoding_type = chunk::DataChunk::EncodingType::kGorilla;
    chunk.encoder.gorilla = storage_.gorilla_encoders.index_of(encoder);
  }
};

}  // namespace series_data