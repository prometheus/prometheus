#pragma once

#include "bare_bones/encoding.h"
#include "bare_bones/zigzag.h"
#include "block.h"

namespace PromPP::Prometheus::Block {

using DataSequence64 = BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec1238>;

class BlocksChunks {
  using SizesSequence = BareBones::EncodedSequence<BareBones::Encoding::RLE<>>;
  using FirstMinTimestampsSequence = BareBones::EncodedSequence<BareBones::Encoding::DeltaZigZagRLE<>>;
  using MinTimestampsSequence = BareBones::EncodedSequence<BareBones::Encoding::RLE<>>;
  using MaxTimestampsSequence = BareBones::EncodedSequence<BareBones::Encoding::RLE<>>;
  using RefsSequence = BareBones::EncodedSequence<BareBones::Encoding::DeltaZigZagRLE<DataSequence64>>;

  Timestamp base_ts_ = 0;

  SizesSequence sizes_;
  FirstMinTimestampsSequence first_min_timestamps_;
  MinTimestampsSequence min_timestamps_;
  MaxTimestampsSequence max_timestamps_;
  RefsSequence refs_;

 public:
  inline __attribute__((always_inline)) explicit BlocksChunks(const Timestamp& base_ts) : base_ts_(base_ts){};

  inline __attribute__((always_inline)) void push_back(const Chunks& chunks) noexcept {
    sizes_.push_back(chunks.size());

    auto chunk_iter = chunks.begin();

    const auto& [min_ts, max_ts, ref] = *chunk_iter;

    assert(std::in_range<uint32_t>(min_ts - base_ts_));
    first_min_timestamps_.push_back(min_ts - base_ts_);

    Timestamp prev_max_ts = max_ts;
    assert(std::in_range<uint32_t>(max_ts - min_ts));
    max_timestamps_.push_back(max_ts - min_ts);

    refs_.push_back(ref);

    ++chunk_iter;

    while (chunk_iter != chunks.end()) {
      const auto& [min_ts, max_ts, ref] = *chunk_iter;

      assert(std::in_range<uint32_t>(min_ts - prev_max_ts));
      min_timestamps_.push_back(min_ts - prev_max_ts);

      prev_max_ts = max_ts;
      assert(std::in_range<uint32_t>(max_ts - min_ts));
      max_timestamps_.push_back(max_ts - min_ts);

      refs_.push_back(ref);

      ++chunk_iter;
    }
  };

  inline __attribute__((always_inline)) void clear() noexcept {
    sizes_.clear();
    first_min_timestamps_.clear();
    min_timestamps_.clear();
    max_timestamps_.clear();
    refs_.clear();
  }

  class ChunkRange {
    uint32_t size_ = 0;
    Timestamp first_min_timestamp_ = 0;

    MinTimestampsSequence::Iterator min_timestamps_i_;
    MaxTimestampsSequence::Iterator max_timestamps_i_;
    RefsSequence::Iterator refs_i_;

   public:
    ChunkRange(uint32_t size,
               Timestamp first_min_timestamp,
               MinTimestampsSequence::Iterator min_timestamps_i,
               MaxTimestampsSequence::Iterator max_timestamps_i,
               RefsSequence::Iterator refs_i)
        : size_(size), first_min_timestamp_(first_min_timestamp), min_timestamps_i_(min_timestamps_i), max_timestamps_i_(max_timestamps_i), refs_i_(refs_i) {}

    class Iterator;

    class IteratorSentinel {
      friend class Iterator;

      uint32_t size_ = 0;

     public:
      inline __attribute__((always_inline)) explicit IteratorSentinel(uint32_t size) : size_(size) {}
    };

    class Iterator {
      uint32_t i_ = 0;
      Timestamp first_min_timestamp_ = 0;
      Timestamp prev_max_timestamp_ = 0;

      MinTimestampsSequence::Iterator min_timestamps_i_;
      MaxTimestampsSequence::Iterator max_timestamps_i_;
      RefsSequence::Iterator refs_i_;

     public:
      Iterator(uint32_t index,
               Timestamp first_min_timestamp,
               Timestamp prev_max_timestamp,
               MinTimestampsSequence::Iterator min_timestamps_i,
               MaxTimestampsSequence::Iterator max_timestamps_i,
               RefsSequence::Iterator refs_i)
          : i_(index),
            first_min_timestamp_(first_min_timestamp),
            prev_max_timestamp_(prev_max_timestamp),
            min_timestamps_i_(min_timestamps_i),
            max_timestamps_i_(max_timestamps_i),
            refs_i_(refs_i) {}

      inline __attribute__((always_inline)) Iterator& operator++() noexcept {
        if (i_ == 0) {
          prev_max_timestamp_ = *max_timestamps_i_ + first_min_timestamp_;
        } else {
          prev_max_timestamp_ = *max_timestamps_i_ + *min_timestamps_i_ + prev_max_timestamp_;
          ++min_timestamps_i_;
        }

        ++max_timestamps_i_;
        ++refs_i_;
        ++i_;

        return *this;
      }

      inline __attribute__((always_inline)) Iterator operator++(int) noexcept {
        Iterator retval = *this;
        ++(*this);
        return retval;
      }

      inline __attribute__((always_inline)) PromPP::Prometheus::Block::Chunk operator*() noexcept {
        if (i_ == 0) {
          return PromPP::Prometheus::Block::Chunk(first_min_timestamp_, *max_timestamps_i_ + first_min_timestamp_, *refs_i_);
        }

        return PromPP::Prometheus::Block::Chunk(*min_timestamps_i_ + prev_max_timestamp_, *max_timestamps_i_ + *min_timestamps_i_ + prev_max_timestamp_,
                                              *refs_i_);
      }

      inline __attribute__((always_inline)) bool operator==(const IteratorSentinel& iter) const { return i_ == iter.size_; }
    };

    inline __attribute__((always_inline)) auto begin() const noexcept {
      return Iterator(0, first_min_timestamp_, 0, min_timestamps_i_, max_timestamps_i_, refs_i_);
    }

    inline __attribute__((always_inline)) auto end() const noexcept { return IteratorSentinel(size_); }

    inline __attribute__((always_inline)) uint32_t size() const noexcept { return size_; }
  };

  class Iterator;

  class IteratorSentinel {
    friend class Iterator;

    SizesSequence::IteratorSentinel sizes_i_s_;

   public:
    inline __attribute__((always_inline)) explicit IteratorSentinel(SizesSequence::IteratorSentinel sizes_i_s) : sizes_i_s_(sizes_i_s) {}
  };

  class Iterator {
    Timestamp base_ts_ = 0;

    SizesSequence::Iterator sizes_i_;
    FirstMinTimestampsSequence::Iterator first_min_timestamps_i_;
    MinTimestampsSequence::Iterator min_timestamps_i_;
    MaxTimestampsSequence::Iterator max_timestamps_i_;
    RefsSequence::Iterator refs_i_;

   public:
    Iterator(Timestamp base_ts,
             SizesSequence::Iterator sizes_i,
             FirstMinTimestampsSequence::Iterator first_min_timestamps_i,
             MinTimestampsSequence::Iterator min_timestamps_i,
             MaxTimestampsSequence::Iterator max_timestamps_i,
             RefsSequence::Iterator refs_i)
        : base_ts_(base_ts),
          sizes_i_(sizes_i),
          first_min_timestamps_i_(first_min_timestamps_i),
          min_timestamps_i_(min_timestamps_i),
          max_timestamps_i_(max_timestamps_i),
          refs_i_(refs_i) {}

    inline __attribute__((always_inline)) Iterator& operator++() noexcept {
      const uint32_t size = *sizes_i_;

      ++sizes_i_;
      ++first_min_timestamps_i_;

      std::ranges::advance(min_timestamps_i_, size - 1);
      std::ranges::advance(max_timestamps_i_, size);
      std::ranges::advance(refs_i_, size);

      return *this;
    }

    inline __attribute__((always_inline)) Iterator operator++(int) noexcept {
      Iterator retval = *this;
      ++(*this);
      return retval;
    }

    inline __attribute__((always_inline)) ChunkRange operator*() noexcept {
      return ChunkRange(*sizes_i_, *first_min_timestamps_i_ + base_ts_, min_timestamps_i_, max_timestamps_i_, refs_i_);
    }

    inline __attribute__((always_inline)) bool operator==(const IteratorSentinel& iter) const { return (this->sizes_i_ == iter.sizes_i_s_); }
  };

  inline __attribute__((always_inline)) auto begin() const noexcept {
    return Iterator(base_ts_, sizes_.begin(), first_min_timestamps_.begin(), min_timestamps_.begin(), max_timestamps_.begin(), refs_.begin());
  }

  inline __attribute__((always_inline)) auto end() const noexcept { return IteratorSentinel(sizes_.end()); }
};
}  // namespace PromPP::Prometheus::Block
