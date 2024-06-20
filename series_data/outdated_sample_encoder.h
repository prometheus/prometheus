#pragma once

#include "data_storage.h"
#include "outdated_chunk_merger.h"

namespace series_data {

template <BareBones::concepts::SystemClockInterface Clock, uint8_t kMaxChunkSize = kSamplesPerChunkDefault>
class OutdatedSampleEncoder {
 public:
  OutdatedSampleEncoder(DataStorage& storage, Clock& clock) : storage_(storage), clock_(clock) {}

  template <EncoderInterface Encoder>
  void encode(Encoder& encoder, uint32_t ls_id, int64_t timestamp, double value) {
    if (auto it = storage_.outdated_chunks.try_emplace(ls_id, clock_, timestamp, value); !it.second) {
      if (it.first->second.encode(timestamp, value) >= kMaxChunkSize) {
        OutdatedChunkMerger<Encoder> merger{storage_, encoder};
        merger.merge(ls_id, it.first->second);
        storage_.outdated_chunks.erase(it.first);
      }
    }
  }

  template <EncoderInterface Encoder>
  void merge_outdated_chunks(Encoder& encoder, std::chrono::seconds ttl) {
    if (storage_.outdated_chunks.empty()) {
      return;
    }

    OutdatedChunkMerger<Encoder> merger{storage_, encoder};
    phmap::erase_if(storage_.outdated_chunks, [ttl, now = clock_.now(), &merger](auto& it) {
      auto& [ls_id, outdated_chunk] = it;
      if (now - outdated_chunk.create_time() >= ttl) {
        merger.merge(ls_id, outdated_chunk);
        return true;
      }

      return false;
    });
  }

 private:
  DataStorage& storage_;
  Clock& clock_;
};

}  // namespace series_data