#pragma once

#include "timestamp.h"
#include "value.h"

namespace series_data::encoder {

struct PROMPP_ATTRIBUTE_PACKED EncoderData {
  uint32_t count{};
  union PROMPP_ATTRIBUTE_PACKED {
    value::Uint32ConstantEncoder uint32_constant;
    uint32_t double_constant_encoder_id;
    uint32_t two_double_constant_encoder_id;
    uint32_t gorilla_encoder_id;
  } values_encoder_data{.double_constant_encoder_id = 0};
  timestamp::StateId timestamp_encoder_state_id{timestamp::kInvalidStateId};
  value::EncodingType values_encoding_type{value::EncodingType::kUnknown};
};

using EncoderDataList = BareBones::Vector<EncoderData>;

}  // namespace series_data::encoder

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::EncoderData> : std::true_type {};

namespace series_data::encoder {

class Encoder {
 public:
  void encode(uint32_t ls_id, int64_t timestamp, double value) {
    auto& data = (encoders_data_.size() > ls_id) ? encoders_data_[ls_id] : encoders_data_.emplace_back();
    data.timestamp_encoder_state_id = timestamp_encoder_.encode(data.timestamp_encoder_state_id, timestamp);

    encode_value(data, value);

    ++data.count;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return encoders_data_.allocated_memory() + gorilla_encoders_.allocated_memory() + double_constant_encoders_.allocated_memory() +
           two_double_constant_encoders_.allocated_memory() + timestamp_encoder_.allocated_memory();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const EncoderDataList& encoders_data() const noexcept { return encoders_data_; }

 public:
  EncoderDataList encoders_data_;
  BareBones::Vector<value::GorillaEncoder> gorilla_encoders_;
  BareBones::VectorWithHoles<value::DoubleConstantEncoder> double_constant_encoders_;
  BareBones::VectorWithHoles<value::TwoDoubleConstantEncoder> two_double_constant_encoders_;
  timestamp::Encoder timestamp_encoder_;

  void encode_value(EncoderData& data, double value) {
    if (data.values_encoding_type == value::EncodingType::kUnknown) {
      [[unlikely]];

      if (value::Uint32ConstantEncoder::can_be_encoded(value)) {
        data.values_encoding_type = value::EncodingType::kUint32Constant;
        new (&data.values_encoder_data) value::Uint32ConstantEncoder(value);
      } else {
        data.values_encoding_type = value::EncodingType::kDoubleConstant;
        auto& encoder = double_constant_encoders_.emplace_back(value);
        data.values_encoder_data.double_constant_encoder_id = double_constant_encoders_.index_of(encoder);
      }
    } else if (data.values_encoding_type == value::EncodingType::kUint32Constant) {
      if (!data.values_encoder_data.uint32_constant.encode(value)) {
        switch_to_two_constant_encoder(data, data.values_encoder_data.uint32_constant.value(), value);
      }
    } else if (data.values_encoding_type == value::EncodingType::kDoubleConstant) {
      if (auto& encoder = double_constant_encoders_[data.values_encoder_data.double_constant_encoder_id]; !encoder.encode(value)) {
        auto encoder_id = data.values_encoder_data.double_constant_encoder_id;
        switch_to_two_constant_encoder(data, encoder.value(), value);
        double_constant_encoders_.erase(encoder_id);
      }
    } else if (data.values_encoding_type == value::EncodingType::kTwoDoubleConstant) {
      if (auto& encoder = two_double_constant_encoders_[data.values_encoder_data.two_double_constant_encoder_id]; !encoder.encode(value)) {
        auto encoder_id = data.values_encoder_data.two_double_constant_encoder_id;
        switch_to_gorilla(data, encoder, value);
        two_double_constant_encoders_.erase(encoder_id);
      }
    } else if (data.values_encoding_type == value::EncodingType::kGorilla) {
      gorilla_encoders_[data.values_encoder_data.gorilla_encoder_id].encode(value);
    }
  }

  PROMPP_ALWAYS_INLINE void switch_to_two_constant_encoder(EncoderData& data, double value1, double value2) {
    auto count = data.count;
    auto& encoder = two_double_constant_encoders_.emplace_back(value1, value2, count);
    data.values_encoding_type = value::EncodingType::kTwoDoubleConstant;
    data.values_encoder_data.two_double_constant_encoder_id = two_double_constant_encoders_.index_of(encoder);
  }

  void switch_to_gorilla(EncoderData& data, const value::TwoDoubleConstantEncoder& encoder, double value) {
    auto& gorilla_encoder = gorilla_encoders_.emplace_back(encoder.value1());
    for (uint32_t i = 1; i < encoder.value1_count(); ++i) {
      gorilla_encoder.encode(encoder.value1());
    }

    auto value2_count = data.count - encoder.value1_count();
    for (uint32_t i = 0; i < value2_count; ++i) {
      gorilla_encoder.encode(encoder.value2());
    }

    gorilla_encoder.encode(value);

    data.values_encoding_type = value::EncodingType::kGorilla;
    data.values_encoder_data.gorilla_encoder_id = gorilla_encoders_.size() - 1;
  }
};

}  // namespace series_data::encoder