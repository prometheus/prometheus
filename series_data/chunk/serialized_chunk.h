#pragma once

#include "bare_bones/vector.h"
#include "data_chunk.h"
#include "primitives/primitives.h"

namespace series_data::chunk {

struct PROMPP_ATTRIBUTE_PACKED SerializedChunk {
  PromPP::Primitives::LabelSetID label_set_id;
  chunk::DataChunk::EncodingType encoding_type{chunk::DataChunk::EncodingType::kUnknown};
  uint32_t values_offset{};
  uint32_t timestamps_offset{};

  explicit SerializedChunk(PromPP::Primitives::LabelSetID _label_set_id) : label_set_id(_label_set_id) {}
};

using SerializedChunkList = BareBones::Vector<SerializedChunk>;

}  // namespace series_data::chunk