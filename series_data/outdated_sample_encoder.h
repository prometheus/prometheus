#pragma once

#include "data_storage.h"
#include "outdated_chunk_merger.h"

namespace series_data {

template <uint8_t kMaxChunkSize = kSamplesPerChunkDefault>
class OutdatedSampleEncoder {
 public:
  explicit OutdatedSampleEncoder(DataStorage& storage) : storage_(storage) {}

  template <EncoderInterface Encoder>
  void encode(Encoder& encoder, uint32_t ls_id, int64_t timestamp, double value) {
    if (auto it = storage_.outdated_chunks_.try_emplace(ls_id, timestamp, value); !it.second) {
      if (it.first->second.encode(timestamp, value) > kMaxChunkSize) {
        OutdatedChunkMerger<Encoder> merger{storage_, encoder};
        merger.merge(ls_id, it.first->second);
      }
    }
  }

 private:
  DataStorage& storage_;
};

}  // namespace series_data