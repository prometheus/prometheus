#pragma once

#include "encoder.h"

namespace series_data::encoder {

class OutdatedChunkMerger {
 public:
  explicit OutdatedChunkMerger(Encoder& encoder) : encoder_(encoder) {}

  void merge() {
    for (auto& [ls_id, encoder] : outdated_chunks_) {
      merge_outdated_chunk(ls_id, encoder);
    }
  }

 private:
  Encoder& encoder_;
};

}  // namespace series_data::encoder