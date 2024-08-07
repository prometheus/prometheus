#pragma once

#include "bare_bones/preprocess.h"
#include "chunk/data_chunk.h"
#include "chunk/finalized_chunk.h"
#include "chunk/outdated_chunk.h"
#include "encoder/gorilla.h"
#include "encoder/value/asc_integer_values_gorilla.h"
#include "encoder/value/double_constant.h"
#include "encoder/value/two_double_constant.h"
#include "encoder/value/values_gorilla.h"
#include "series_data/encoder/timestamp/encoder.h"

namespace series_data {

struct DataStorage {
 private:
  class IteratorSentinel {};

  class SeriesChunkIterator {
   public:
    class Data {
     public:
      explicit Data(const DataStorage* storage, uint32_t ls_id) : storage_(storage) {
        if (storage_->open_chunks.size() > ls_id) {
          open_chunk_ = &storage_->open_chunks[ls_id];

          if (auto it = storage_->finalized_chunks.find(ls_id); it != storage_->finalized_chunks.end()) {
            finalized_chunk_iterator_ = it->second.begin();
            finalized_chunk_end_iterator_ = it->second.end();
          }
        }
      }

      [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t ls_id() const noexcept { return std::distance(storage_->open_chunks.begin(), open_chunk_); }
      [[nodiscard]] PROMPP_ALWAYS_INLINE chunk::DataChunk::Type chunk_type() const noexcept {
        return finalized_chunk_iterator_ == finalized_chunk_end_iterator_ ? chunk::DataChunk::Type::kOpen : chunk::DataChunk::Type::kFinalized;
      }
      [[nodiscard]] PROMPP_ALWAYS_INLINE const chunk::DataChunk& chunk() const noexcept {
        return chunk_type() == chunk::DataChunk::Type::kOpen ? *open_chunk_ : *finalized_chunk_iterator_;
      }

     private:
      friend class SeriesChunkIterator;

      const DataStorage* storage_;
      std::forward_list<chunk::DataChunk>::const_iterator finalized_chunk_iterator_;
      std::forward_list<chunk::DataChunk>::const_iterator finalized_chunk_end_iterator_;
      const chunk::DataChunk* open_chunk_{};

      PROMPP_ALWAYS_INLINE void next_value() noexcept {
        if (finalized_chunk_iterator_ != finalized_chunk_end_iterator_) {
          ++finalized_chunk_iterator_;
          return;
        }

        open_chunk_ = nullptr;
      }

      [[nodiscard]] PROMPP_ALWAYS_INLINE bool has_value() const noexcept { return open_chunk_ != nullptr; }
    };

    using iterator_category = std::forward_iterator_tag;
    using value_type = Data;
    using difference_type = ptrdiff_t;
    using pointer = Data*;
    using reference = Data&;

    explicit SeriesChunkIterator(const DataStorage* data_storage, uint32_t ls_id) : data_(data_storage, ls_id) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE const Data& operator*() const noexcept { return data_; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE const Data* operator->() const noexcept { return &data_; }

    PROMPP_ALWAYS_INLINE SeriesChunkIterator& operator++() noexcept {
      data_.next_value();
      return *this;
    }

    PROMPP_ALWAYS_INLINE SeriesChunkIterator operator++(int) noexcept {
      auto it = *this;
      ++*this;
      return it;
    }

    PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return !data_.has_value(); }

   private:
    Data data_;
  };

  class ChunkIterator {
   public:
    using iterator_category = std::forward_iterator_tag;
    using value_type = SeriesChunkIterator::Data;
    using difference_type = ptrdiff_t;
    using pointer = SeriesChunkIterator::Data*;
    using reference = SeriesChunkIterator::Data&;

    explicit ChunkIterator(const DataStorage* storage) : storage_(storage), iterator_(storage->open_chunks.begin()), series_chunk_iterator_(storage, 0U) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesChunkIterator::Data& operator*() const noexcept { return *series_chunk_iterator_; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesChunkIterator::Data* operator->() const noexcept { return series_chunk_iterator_.operator->(); }

    PROMPP_ALWAYS_INLINE ChunkIterator& operator++() noexcept {
      if (++series_chunk_iterator_ == IteratorSentinel{}) {
        if (++iterator_ != storage_->open_chunks.end()) {
          series_chunk_iterator_ = SeriesChunkIterator{storage_, static_cast<uint32_t>(std::distance(storage_->open_chunks.begin(), iterator_))};
        }
      }
      return *this;
    }

    PROMPP_ALWAYS_INLINE ChunkIterator operator++(int) noexcept {
      auto it = *this;
      ++*this;
      return it;
    }

    PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept {
      return iterator_ == storage_->open_chunks.end() && series_chunk_iterator_ == IteratorSentinel{};
    }

   private:
    const DataStorage* storage_;
    BareBones::Vector<chunk::DataChunk>::const_iterator iterator_;
    SeriesChunkIterator series_chunk_iterator_;
  };

  class SeriesChunks {
   public:
    explicit SeriesChunks(const DataStorage* storage, uint32_t series_id) : storage_(storage), series_id_(series_id) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE SeriesChunkIterator begin() const noexcept { return SeriesChunkIterator{storage_, series_id_}; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

   private:
    const DataStorage* storage_;
    const uint32_t series_id_;
  };

  class Chunks {
   public:
    explicit Chunks(const DataStorage* storage) : storage_(storage) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE ChunkIterator begin() const noexcept { return ChunkIterator{storage_}; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

   private:
    const DataStorage* storage_;
  };

 public:
  BareBones::Vector<chunk::DataChunk> open_chunks;
  encoder::timestamp::Encoder timestamp_encoder;

  BareBones::VectorWithHoles<encoder::value::DoubleConstantEncoder> double_constant_encoders;
  BareBones::VectorWithHoles<encoder::value::TwoDoubleConstantEncoder> two_double_constant_encoders;
  BareBones::VectorWithHoles<encoder::value::AscIntegerValuesGorillaEncoder> asc_integer_values_gorilla_encoders;
  BareBones::VectorWithHoles<encoder::value::ValuesGorillaEncoder> values_gorilla_encoders;
  BareBones::VectorWithHoles<encoder::GorillaEncoder> gorilla_encoders;

  size_t outdated_chunks_map_allocated_memory{};
  phmap::
      flat_hash_map<uint32_t, chunk::OutdatedChunk, std::hash<uint32_t>, std::equal_to<>, BareBones::Allocator<std::pair<const uint32_t, chunk::OutdatedChunk>>>
          outdated_chunks{{}, {}, BareBones::Allocator<std::pair<const uint32_t, chunk::OutdatedChunk>>{outdated_chunks_map_allocated_memory}};

  BareBones::VectorWithHoles<encoder::RefCountableBitSequenceWithItemsCount> finalized_timestamp_streams;
  BareBones::VectorWithHoles<encoder::CompactBitSequence> finalized_data_streams;
  size_t finalized_chunks_map_allocated_memory{};
  phmap::flat_hash_map<uint32_t,
                       chunk::FinalizedChunkList,
                       std::hash<uint32_t>,
                       std::equal_to<>,
                       BareBones::Allocator<std::pair<const uint32_t, std::forward_list<chunk::DataChunk>>>>
      finalized_chunks{{}, {}, BareBones::Allocator<std::pair<const uint32_t, std::forward_list<chunk::DataChunk>>>{finalized_chunks_map_allocated_memory}};

  uint32_t samples_count{};
  uint32_t outdated_samples_count{};
  uint32_t outdated_chunks_count{};
  uint32_t merged_samples_count{};

  [[nodiscard]] PROMPP_ALWAYS_INLINE SeriesChunks chunks(uint32_t ls_id) const noexcept { return SeriesChunks{this, ls_id}; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE Chunks chunks() const noexcept { return Chunks{this}; }

  void reset_sample_counters() noexcept {
    samples_count = 0;
    outdated_samples_count = 0;
    merged_samples_count = 0;
  }

  template <chunk::DataChunk::Type chunk_type>
  void erase_chunk(const chunk::DataChunk& chunk) {
    if (chunk.encoding_type != chunk::DataChunk::EncodingType::kGorilla) {
      erase_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id);
    }

    erase_encoder_data<chunk_type>(chunk);
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE const encoder::BitSequenceWithItemsCount& get_timestamp_stream(uint32_t stream_id) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return timestamp_encoder.get_stream(stream_id);
    } else {
      return finalized_timestamp_streams[stream_id].stream;
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE const encoder::CompactBitSequence& get_asc_integer_values_gorilla_stream(uint32_t stream_id) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return asc_integer_values_gorilla_encoders[stream_id].stream();
    } else {
      return finalized_data_streams[stream_id];
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE const encoder::CompactBitSequence& get_values_gorilla_stream(uint32_t stream_id) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return values_gorilla_encoders[stream_id].stream();
    } else {
      return finalized_data_streams[stream_id];
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE const encoder::CompactBitSequence& get_gorilla_encoder_stream(uint32_t stream_id) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return gorilla_encoders[stream_id].stream().stream;
    } else {
      return finalized_data_streams[stream_id];
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    size_t outdated_chunks_allocated_memory = 0;
    for (auto& [_, outdated_chunk] : outdated_chunks) {
      outdated_chunks_allocated_memory += outdated_chunk.allocated_memory();
    }

    return open_chunks.allocated_memory() + double_constant_encoders.allocated_memory() + two_double_constant_encoders.allocated_memory() +
           asc_integer_values_gorilla_encoders.allocated_memory() + values_gorilla_encoders.allocated_memory() + gorilla_encoders.allocated_memory() +
           timestamp_encoder.allocated_memory() + finalized_timestamp_streams.allocated_memory() + finalized_data_streams.allocated_memory() +
           finalized_chunks_map_allocated_memory + outdated_chunks_map_allocated_memory + outdated_chunks_allocated_memory;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory(chunk::DataChunk::EncodingType type) const noexcept {
    using enum chunk::DataChunk::EncodingType;

    switch (type) {
      case kDoubleConstant: {
        return double_constant_encoders.allocated_memory();
      }

      case kTwoDoubleConstant: {
        return two_double_constant_encoders.allocated_memory();
      }

      case kAscIntegerValuesGorilla: {
        return asc_integer_values_gorilla_encoders.allocated_memory();
      }

      case kValuesGorilla: {
        return values_gorilla_encoders.allocated_memory();
      }

      case kGorilla: {
        return gorilla_encoders.allocated_memory();
      }

      case kUint32Constant:
      case kUnknown: {
        return 0;
      }

      default: {
        assert(type != kUnknown);
        return 0;
      }
    }
  }

 private:
  template <chunk::DataChunk::Type chunk_type>
  void erase_timestamp_stream(uint32_t stream_id) {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      timestamp_encoder.erase(stream_id);
    } else {
      if (--finalized_timestamp_streams[stream_id].reference_count == 0) {
        finalized_timestamp_streams.erase(stream_id);
      }
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  void erase_encoder_data(const chunk::DataChunk& chunk) {
    using enum chunk::DataChunk::EncodingType;

    switch (chunk.encoding_type) {
      case kDoubleConstant: {
        double_constant_encoders.erase(chunk.encoder.double_constant);
        break;
      }

      case kTwoDoubleConstant: {
        two_double_constant_encoders.erase(chunk.encoder.two_double_constant);
        break;
      }

      case kAscIntegerValuesGorilla: {
        if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
          asc_integer_values_gorilla_encoders.erase(chunk.encoder.asc_integer_values_gorilla);
        } else {
          finalized_data_streams.erase(chunk.encoder.asc_integer_values_gorilla);
        }
        break;
      }

      case kValuesGorilla: {
        if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
          values_gorilla_encoders.erase(chunk.encoder.values_gorilla);
        } else {
          finalized_data_streams.erase(chunk.encoder.values_gorilla);
        }
        break;
      }

      case kGorilla: {
        if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
          gorilla_encoders.erase(chunk.encoder.gorilla);
        } else {
          finalized_data_streams.erase(chunk.encoder.gorilla);
        }
        break;
      }

      case kUint32Constant:
      case kUnknown: {
        break;
      }
    }
  }
};

}  // namespace series_data