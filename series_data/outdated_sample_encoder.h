#pragma once

#include "data_storage.h"
#include "outdated_chunk_merger.h"

namespace series_data {

template <BareBones::concepts::SystemClockInterface Clock, uint8_t kMaxChunkSize = kSamplesPerChunkDefault>
class OutdatedSampleEncoder {
 public:
  explicit OutdatedSampleEncoder(Clock& clock) : clock_(clock) {}

  template <EncoderInterface Encoder>
  void encode(Encoder& encoder, uint32_t ls_id, int64_t timestamp, double value) {
    auto& storage = encoder.storage();
    ++storage.outdated_samples_count;

    if (auto it = storage.outdated_chunks.try_emplace(ls_id, clock_, timestamp, value); !it.second) {
      if (it.first->second.encode(timestamp, value) >= kMaxChunkSize) {
        OutdatedChunkMerger<Encoder> merger{encoder};
        merger.merge(ls_id, it.first->second);
        storage.outdated_chunks.erase(it.first);
      }
    } else {
      ++storage.outdated_chunks_count;
    }
  }

  template <EncoderInterface Encoder>
  void merge_outdated_chunks(Encoder& encoder, std::chrono::seconds ttl) {
    if (encoder.storage().outdated_chunks.empty()) {
      return;
    }

    OutdatedChunkMerger<Encoder> merger{encoder};
    phmap::erase_if(encoder.storage().outdated_chunks, [ttl, now = clock_.now(), &merger](auto& it) {
      if (auto& [ls_id, outdated_chunk] = it; now - outdated_chunk.create_time() >= ttl) {
        merger.merge(ls_id, outdated_chunk);
        return true;
      }

      return false;
    });
  }

 private:
  Clock& clock_;
};

}  // namespace series_data