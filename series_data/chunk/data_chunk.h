#pragma once

#include "bare_bones/preprocess.h"
#include "series_data/encoder/timestamp/state.h"
#include "series_data/encoder/value/uint32_constant.h"

namespace series_data::chunk {

struct PROMPP_ATTRIBUTE_PACKED DataChunk {
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
  };

  EncoderData encoder{.double_constant = 0};
  encoder::timestamp::State::Id timestamp_encoder_state_id{encoder::timestamp::State::kInvalidId};
  EncodingType encoding_type{EncodingType::kUnknown};

  DataChunk() = default;
  DataChunk(const DataChunk& other, encoder::timestamp::State::Id finalized_stream_id)
      : encoder{.double_constant = other.encoder.double_constant}, timestamp_encoder_state_id(finalized_stream_id), encoding_type(other.encoding_type) {}

  PROMPP_ALWAYS_INLINE void reset() noexcept {
    encoder.double_constant = 0;
    timestamp_encoder_state_id = encoder::timestamp::State::kInvalidId;
    encoding_type = EncodingType::kUnknown;
  }
};

}  // namespace series_data::chunk