#pragma once

#include <forward_list>

#include "bare_bones/preprocess.h"

namespace series_data::chunk {

class FinalizedChunkList {
 public:
  using ChunksList = std::forward_list<DataChunk, BareBones::Allocator<DataChunk>>;

  explicit FinalizedChunkList(size_t& allocated_memory_) : chunks_(BareBones::Allocator<DataChunk>{allocated_memory_}) {}

  template <class GetFinalizedChunkFirstTimestamp>
  DataChunk& emplace(DataChunk& chunk, GetFinalizedChunkFirstTimestamp&& get_finalized_chunk_first_timestamp) {
    auto chunk_first_timestamp = get_finalized_chunk_first_timestamp(chunk);

    auto it = chunks_.before_begin();
    for (auto next_it = it;; it = next_it) {
      if (++next_it == chunks_.end() || chunk_first_timestamp < get_finalized_chunk_first_timestamp(*next_it)) {
        return *chunks_.emplace_after(it, chunk);
      }
    }
  }

  void erase(const DataChunk& chunk) noexcept {
    auto it = chunks_.before_begin();
    for (auto next_it = it;; it = next_it) {
      if (&*++next_it == &chunk) {
        chunks_.erase_after(it);
        return;
      }
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE ChunksList::const_iterator begin() const noexcept { return chunks_.begin(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const DataChunk& front() const noexcept { return chunks_.front(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE ChunksList::const_iterator end() const noexcept { return chunks_.end(); }

 private:
  ChunksList chunks_;
};

}  // namespace series_data::chunk