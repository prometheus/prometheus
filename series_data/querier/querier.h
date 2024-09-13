#pragma once

#include "query.h"
#include "series_data/data_storage.h"
#include "series_data/decoder.h"

namespace series_data::querier {

class Querier {
 public:
  explicit Querier(const DataStorage& storage) : storage_(storage) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE const QueriedChunkList& query(const Query& query) {
    chunks_.clear();

    for (auto& ls_id : query.label_set_ids) {
      query_chunks(ls_id, query.start_timestamp_ms, query.end_timestamp_ms);
    }

    return chunks_;
  }

 private:
  using ChunkType = chunk::DataChunk::Type;

  const DataStorage& storage_;
  QueriedChunkList chunks_;

  PROMPP_ALWAYS_INLINE void query_chunks(PromPP::Primitives::LabelSetID ls_id, int64_t start_timestamp_ms, int64_t end_timestamp_ms) {
    query_finalized_chunks(ls_id, start_timestamp_ms, end_timestamp_ms);
    query_opened_chunks(ls_id, start_timestamp_ms, end_timestamp_ms);
  }

  void query_finalized_chunks(PromPP::Primitives::LabelSetID ls_id, int64_t start_timestamp_ms, int64_t end_timestamp_ms) {
    if (auto it = storage_.finalized_chunks.find(ls_id); it != storage_.finalized_chunks.end()) {
      uint32_t finalized_chunk_index = 0;
      auto& finalized_chunks = it->second;
      for (auto chunk_it = finalized_chunks.begin(); chunk_it != finalized_chunks.end(); ++chunk_it, ++finalized_chunk_index) {
        auto chunk_start_timestamp_ms = Decoder::get_chunk_first_timestamp<ChunkType::kFinalized>(storage_, *chunk_it);
        if (chunk_start_timestamp_ms > end_timestamp_ms) {
          return;
        }

        if (is_intersect(start_timestamp_ms, chunk_start_timestamp_ms, end_timestamp_ms,
                         get_finalized_chunk_last_timestamp(finalized_chunks, chunk_it, ls_id))) {
          chunks_.emplace_back(ls_id, finalized_chunk_index);
        }
      }
    }
  }

  void query_opened_chunks(PromPP::Primitives::LabelSetID ls_id, int64_t start_timestamp_ms, int64_t end_timestamp_ms) {
    if (storage_.open_chunks.size() > ls_id) {
      if (auto& open_chunk = storage_.open_chunks[ls_id]; !open_chunk.is_empty()) {
        auto chunk_start_timestamp_ms = Decoder::get_chunk_first_timestamp<ChunkType::kOpen>(storage_, open_chunk);
        if (chunk_start_timestamp_ms > end_timestamp_ms) {
          return;
        }

        if (is_intersect(start_timestamp_ms, chunk_start_timestamp_ms, end_timestamp_ms, Decoder::get_open_chunk_last_timestamp(storage_, open_chunk))) {
          chunks_.emplace_back(ls_id);
        }
      }
    }
  }

  PROMPP_ALWAYS_INLINE static bool is_intersect(int64_t start1, int64_t start2, int64_t end1, int64_t end2) noexcept {
    return std::max(start1, start2) <= std::min(end1, end2);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t get_finalized_chunk_last_timestamp(const chunk::FinalizedChunkList& finalized_chunks,
                                                                                chunk::FinalizedChunkList::ChunksList::const_iterator chunk_it,
                                                                                PromPP::Primitives::LabelSetID ls_id) const noexcept {
    if (auto next_chunk_it = std::next(chunk_it); next_chunk_it != finalized_chunks.end()) {
      return Decoder::get_chunk_first_timestamp<ChunkType::kFinalized>(storage_, *next_chunk_it) - 1;
    }

    return Decoder::get_chunk_first_timestamp<ChunkType::kOpen>(storage_, storage_.open_chunks[ls_id]) - 1;
  }
};

}  // namespace series_data::querier