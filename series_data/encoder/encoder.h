#pragma once

#include "timestamp.h"
#include "value.h"

namespace series_data::encoder {

enum class ChunkType : uint8_t {
  kUnknown,
  kUint32Constant,
  kDoubleConstant,
  kTwoDoubleConstant,
  kValuesGorilla,
};

struct PROMPP_ATTRIBUTE_PACKED DataChunk {
  union PROMPP_ATTRIBUTE_PACKED {
    value::Uint32ConstantEncoder uint32_constant;
    uint32_t double_constant;
    uint32_t two_double_constant;
    uint32_t values_gorilla;
  } encoder{.double_constant = 0};
  timestamp::StateId timestamp_encoder_state_id{timestamp::kInvalidStateId};
  ChunkType type{ChunkType::kUnknown};
};

}  // namespace series_data::encoder

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::DataChunk> : std::true_type {};

namespace series_data::encoder {

class Encoder {
 public:
  void encode(uint32_t ls_id, int64_t timestamp, double value) {
    auto& chunk = (encoders_data_.size() > ls_id) ? encoders_data_[ls_id] : encoders_data_.emplace_back();
    chunk.timestamp_encoder_state_id = timestamp_encoder_.encode(chunk.timestamp_encoder_state_id, timestamp);

    encode_value(chunk, value);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return encoders_data_.allocated_memory() + double_constant_encoders_.allocated_memory() + two_double_constant_encoders_.allocated_memory() +
           values_gorilla_encoders_.allocated_memory() + timestamp_encoder_.allocated_memory();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const BareBones::Vector<DataChunk>& encoders_data() const noexcept { return encoders_data_; }

 public:
  BareBones::Vector<DataChunk> encoders_data_;
  BareBones::VectorWithHoles<value::DoubleConstantEncoder> double_constant_encoders_;
  BareBones::VectorWithHoles<value::TwoDoubleConstantEncoder> two_double_constant_encoders_;
  BareBones::Vector<value::ValuesGorillaEncoder> values_gorilla_encoders_;
  timestamp::Encoder timestamp_encoder_;

  void encode_value(DataChunk& chunk, double value) {
    if (chunk.type == ChunkType::kUnknown) {
      [[unlikely]];

      if (value::Uint32ConstantEncoder::can_be_encoded(value)) {
        chunk.type = ChunkType::kUint32Constant;
        new (&chunk.encoder) value::Uint32ConstantEncoder(value);
      } else {
        chunk.type = ChunkType::kDoubleConstant;
        auto& encoder = double_constant_encoders_.emplace_back(value);
        chunk.encoder.double_constant = double_constant_encoders_.index_of(encoder);
      }
    } else if (chunk.type == ChunkType::kUint32Constant) {
      if (!chunk.encoder.uint32_constant.encode(value)) {
        switch_to_two_constant_encoder(chunk, chunk.encoder.uint32_constant.value(), value);
      }
    } else if (chunk.type == ChunkType::kDoubleConstant) {
      if (auto& encoder = double_constant_encoders_[chunk.encoder.double_constant]; !encoder.encode(value)) {
        auto encoder_id = chunk.encoder.double_constant;
        switch_to_two_constant_encoder(chunk, encoder.value(), value);
        double_constant_encoders_.erase(encoder_id);
      }
    } else if (chunk.type == ChunkType::kTwoDoubleConstant) {
      if (auto& encoder = two_double_constant_encoders_[chunk.encoder.two_double_constant]; !encoder.encode(value)) {
        auto encoder_id = chunk.encoder.two_double_constant;
        switch_to_values_gorilla(chunk, encoder, value);
        two_double_constant_encoders_.erase(encoder_id);
      }
    } else if (chunk.type == ChunkType::kValuesGorilla) {
      values_gorilla_encoders_[chunk.encoder.values_gorilla].encode(value);
    }
  }

  PROMPP_ALWAYS_INLINE void switch_to_two_constant_encoder(DataChunk& chunk, double value1, double value2) {
    auto& encoder = two_double_constant_encoders_.emplace_back(value1, value2, timestamp_encoder_.get_encoder(chunk.timestamp_encoder_state_id).count() - 1);
    chunk.type = ChunkType::kTwoDoubleConstant;
    chunk.encoder.two_double_constant = two_double_constant_encoders_.index_of(encoder);
  }

  void switch_to_values_gorilla(DataChunk& data, const value::TwoDoubleConstantEncoder& encoder, double value) {
    auto& gorilla_encoder = values_gorilla_encoders_.emplace_back(encoder.value1());
    for (uint32_t i = 1; i < encoder.value1_count(); ++i) {
      gorilla_encoder.encode(encoder.value1());
    }

    auto value2_count = timestamp_encoder_.get_encoder(data.timestamp_encoder_state_id).count() - encoder.value1_count() - 1;
    for (uint32_t i = 0; i < value2_count; ++i) {
      gorilla_encoder.encode(encoder.value2());
    }

    gorilla_encoder.encode(value);

    data.type = ChunkType::kValuesGorilla;
    data.encoder.values_gorilla = values_gorilla_encoders_.size() - 1;
  }
};

}  // namespace series_data::encoder