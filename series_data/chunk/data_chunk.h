#pragma once

#include "bare_bones/preprocess.h"
#include "series_data/encoder/timestamp/state.h"
#include "series_data/encoder/value/uint32_constant.h"

namespace series_data::chunk {

struct PROMPP_ATTRIBUTE_PACKED DataChunk {
  enum class Type : uint8_t {
    kOpen = 0,
    kFinalized,
  };

  enum class EncodingType : uint8_t {
    kUnknown,
    kUint32Constant,
    kDoubleConstant,
    kTwoDoubleConstant,
    kAscIntegerValuesGorilla,
    kValuesGorilla,
    kGorilla,
  };

  union PROMPP_ATTRIBUTE_PACKED EncoderData {
    encoder::value::Uint32ConstantEncoder uint32_constant;
    uint32_t double_constant;
    uint32_t two_double_constant;
    uint32_t asc_integer_values_gorilla;
    uint32_t values_gorilla;
    uint32_t gorilla;

    PROMPP_ALWAYS_INLINE bool operator==(const EncoderData& other) const noexcept { return double_constant == other.double_constant; }
  };

  EncoderData encoder{.double_constant = 0};
  encoder::timestamp::State::Id timestamp_encoder_state_id{encoder::timestamp::State::kInvalidId};
  EncodingType encoding_type{EncodingType::kUnknown};

  DataChunk() = default;
  DataChunk(const DataChunk&) noexcept = default;

  DataChunk(uint32_t encoder_id, encoder::timestamp::State::Id _timestamp_encoder_state_id, EncodingType _encoding_type)
      : encoder{.double_constant = encoder_id}, timestamp_encoder_state_id(_timestamp_encoder_state_id), encoding_type(_encoding_type) {}

  DataChunk& operator=(const DataChunk& other) noexcept {
    if (this != &other) {
      encoder.double_constant = other.encoder.double_constant;
      timestamp_encoder_state_id = other.timestamp_encoder_state_id;
      encoding_type = other.encoding_type;
    }

    return *this;
  }

  bool operator==(const DataChunk&) const noexcept = default;

  PROMPP_ALWAYS_INLINE void reset() noexcept {
    encoder.double_constant = 0;
    timestamp_encoder_state_id = encoder::timestamp::State::kInvalidId;
    encoding_type = EncodingType::kUnknown;
  }
};

}  // namespace series_data::chunk

template <>
struct BareBones::IsTriviallyReallocatable<series_data::chunk::DataChunk> : std::true_type {};