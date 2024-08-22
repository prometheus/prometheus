#pragma once

#include <roaring/roaring.hh>

#include "bare_bones/preprocess.h"
#include "types.h"

namespace PromPP::Primitives::lss_metadata {

class ChangesCollector {
 public:
  PROMPP_ALWAYS_INLINE void add(uint32_t name_id) { changed_metadata_.add(name_id); }

  PROMPP_ALWAYS_INLINE void add(uint32_t start_name_id, uint32_t end_name_id) {
    for (; start_name_id < end_name_id; ++start_name_id) {
      changed_metadata_.add(start_name_id);
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const SymbolsTableCheckpoint& symbols_table_checkpoint() const noexcept { return symbols_table_checkpoint_; }

  PROMPP_ALWAYS_INLINE void reset(const SymbolsTableCheckpoint& symbols_table_checkpoint) noexcept {
    changed_metadata_ = roaring::Roaring{};
    symbols_table_checkpoint_ = symbols_table_checkpoint;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE roaring::RoaringSetBitForwardIterator begin() const noexcept { return changed_metadata_.begin(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE roaring::RoaringSetBitForwardIterator end() const noexcept { return changed_metadata_.end(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t count() const noexcept { return changed_metadata_.cardinality(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t allocated_memory() const noexcept { return changed_metadata_.getSizeInBytes(); }

 private:
  roaring::Roaring changed_metadata_;
  SymbolsTableCheckpoint symbols_table_checkpoint_;
};

class NopChangesCollector {
 public:
  struct IteratorSentinel {
    IteratorSentinel& operator++() noexcept { return *this; }
    uint32_t operator*() const noexcept { return 0; }
  };

  static void add(uint32_t) {}
  static void add(uint32_t, uint32_t) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE static const SymbolsTableCheckpoint& symbols_table_checkpoint() noexcept {
    static SymbolsTableCheckpoint symbols_table_checkpoint;
    return symbols_table_checkpoint;
  }

  static void reset(const SymbolsTableCheckpoint&) noexcept {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel begin() noexcept { return {}; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static uint32_t count() noexcept { return 0U; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static uint32_t allocated_memory() noexcept { return 0; }
};

}  // namespace PromPP::Primitives::lss_metadata