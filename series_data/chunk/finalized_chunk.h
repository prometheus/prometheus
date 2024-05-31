#pragma once

#include <forward_list>

#include "bare_bones/preprocess.h"
#include "data_chunk.h"

namespace series_data::chunk {

class FinalizedChunkList {
 public:
  using ChunksList = std::forward_list<DataChunk, BareBones::Allocator<DataChunk>>;

  explicit FinalizedChunkList(size_t& allocated_memory_) : chunks_(BareBones::Allocator<DataChunk>{allocated_memory_}) {}

  template <class... Args>
  DataChunk& emplace_back(Args&&... args) {
    for (auto it = chunks_.before_begin(), next_it = it;; it = next_it) {
      if (++next_it == chunks_.end()) {
        return *chunks_.emplace_after(it, std::forward<Args>(args)...);
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