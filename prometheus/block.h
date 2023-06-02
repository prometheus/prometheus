#pragma once

#include <cstdint>
#include <fstream>
#include <limits>
#include <tuple>
#include <vector>

#include "bare_bones/fio.h"
#include "bare_bones/vector.h"
#include "primitives/primitives.h"
#include "primitives/snug_composites.h"

namespace PromPP::Prometheus::Block {

using SymbolsTable = BareBones::SnugComposite::OrderedDecodingTable<PromPP::Primitives::SnugComposites::Filaments::Symbol>;
using LabelViewSet = PromPP::Primitives::LabelViewSet;
using Timestamp = PromPP::Primitives::Timestamp;
using ChunkRef = uint64_t;
using Chunk = std::tuple<Timestamp, Timestamp, ChunkRef>;

class Chunks {
  BareBones::Vector<Chunk> data_;

 public:
  using Iterator = BareBones::Vector<Chunk>::const_iterator;

  Chunks() noexcept = default;
  Chunks(const Chunks&) noexcept = default;
  Chunks& operator=(const Chunks&) noexcept = default;
  Chunks(Chunks&&) noexcept = default;
  Chunks& operator=(Chunks&&) noexcept = default;

  void add(Timestamp minTime, Timestamp maxTime, ChunkRef ref) noexcept { data_.push_back({minTime, maxTime, ref}); }

  size_t size() const noexcept { return data_.size(); }

  void clear() noexcept { data_.clear(); }

  Iterator begin() const noexcept { return std::begin(data_); }
  Iterator end() const noexcept { return std::end(data_); }
};

}  // namespace PromPP::Prometheus::Block

namespace BareBones {

template <>
struct IsTriviallyReallocatable<PromPP::Prometheus::Block::Chunks> : std::true_type {};

template <>
struct IsZeroInitializable<PromPP::Prometheus::Block::Chunks> : std::true_type {};

}  // namespace BareBones

namespace PromPP::Prometheus::Block {

template <class LabelSetType, class ChunksType = Chunks>
class BasicSeries {
  LabelSetType label_set_;
  ChunksType chunks_;

 public:
  using label_set_type = LabelSetType;
  using chunks_type = ChunksType;

  BasicSeries() noexcept = default;
  BasicSeries(const BasicSeries&) noexcept = default;
  BasicSeries& operator=(const BasicSeries&) noexcept = default;
  BasicSeries(BasicSeries&&) noexcept = default;
  BasicSeries& operator=(BasicSeries&&) noexcept = default;

  BasicSeries(const LabelSetType& label_set, const ChunksType chunks) noexcept : label_set_(label_set), chunks_(chunks) {}

  inline __attribute__((always_inline)) auto& label_set() noexcept {
    if constexpr (std::is_pointer<LabelSetType>::value) {
      return *label_set_;
    } else {
      return label_set_;
    }
  }

  inline __attribute__((always_inline)) const auto& label_set() const noexcept {
    if constexpr (std::is_pointer<LabelSetType>::value) {
      return *label_set_;
    } else {
      return label_set_;
    }
  }

  inline __attribute__((always_inline)) void set_label_set(LabelSetType label_set) noexcept {
    static_assert(std::is_pointer<LabelSetType>::value, "this functions can be used only if LabelSetType is a pointer");
    label_set_ = label_set;
  }

  inline __attribute__((always_inline)) const auto& chunks() const noexcept {
    if constexpr (std::is_pointer<ChunksType>::value) {
      return *chunks_;
    } else {
      return chunks_;
    }
  }

  inline __attribute__((always_inline)) auto& chunks() noexcept {
    if constexpr (std::is_pointer<ChunksType>::value) {
      return *chunks_;
    } else {
      return chunks_;
    }
  }

  inline __attribute__((always_inline)) void set_label_set(ChunksType chunks) noexcept {
    static_assert(std::is_pointer<ChunksType>::value, "this functions can be used only if ChunksType is a pointer");
    chunks_ = chunks;
  }

  inline __attribute__((always_inline)) void clear() noexcept {
    label_set().clear();
    chunks().clear();
  }
};

using Series = BasicSeries<LabelViewSet, Chunks>;

class OrderedSeriesList {
  Primitives::SnugComposites::LabelSet::OrderedIndexingTable label_sets_;
  BareBones::Vector<Chunks> chunks_;

 public:
  size_t size() const noexcept { return label_sets_.size(); }

  const auto& label_sets() const { return label_sets_; }

  template <class SeriesType>
  void push_back(const SeriesType& series) noexcept {
    label_sets_.emplace_back(series.label_set());
    chunks_.emplace_back(series.chunks());
  }

  class IteratorSentinel {};

  class Iterator {
    const OrderedSeriesList* ordered_series_list_;
    Primitives::LabelSetID i_ = 0;

   public:
    using iterator_category = std::forward_iterator_tag;
    using value_type = Series;
    using difference_type = std::ptrdiff_t;

    inline __attribute__((always_inline)) explicit Iterator(const OrderedSeriesList* ordered_series_list) noexcept
        : ordered_series_list_(ordered_series_list) {}

    inline __attribute__((always_inline)) Iterator& operator++() noexcept {
      ++i_;
      return *this;
    }

    inline __attribute__((always_inline)) Iterator operator++(int) noexcept {
      Iterator retval = *this;
      ++(*this);
      return retval;
    }

    inline __attribute__((always_inline)) bool operator==(const IteratorSentinel& other) const noexcept { return i_ == ordered_series_list_->size(); }

    inline __attribute__((always_inline)) auto operator*() const noexcept {
      return BasicSeries<const Primitives::SnugComposites::LabelSet::OrderedIndexingTable::value_type, const Chunks*>(ordered_series_list_->label_sets_[i_],
                                                                                                                      &ordered_series_list_->chunks_[i_]);
    }
  };

  using const_iterator = Iterator;
  inline __attribute__((always_inline)) auto begin() const noexcept { return Iterator(this); }

  inline __attribute__((always_inline)) auto end() const noexcept { return IteratorSentinel(); }
};

template <typename Reader>
class TableOfContent {
 public:
  static constexpr uint64_t SIZE_OF_TABLE_OF_CONTENT_IN_BYTES = 52;

 public:
  explicit TableOfContent(Reader& r) {
    reference_to_symbols_table_ = r.template get_big_endian<uint64_t>();
    reference_to_series_table_ = r.template get_big_endian<uint64_t>();
    reference_to_label_indices_table_ = r.template get_big_endian<uint64_t>();
  }
  TableOfContent(const TableOfContent&) = default;
  TableOfContent& operator=(const TableOfContent&) = default;
  TableOfContent(TableOfContent&&) = default;
  TableOfContent& operator=(TableOfContent&&) = default;

  uint64_t symbols_table_offset() const { return reference_to_symbols_table_; }
  uint64_t symbols_table_length() const { return reference_to_series_table_ - reference_to_symbols_table_; }
  uint64_t series_table_offset() const { return reference_to_series_table_; }
  uint64_t series_table_length() const { return reference_to_label_indices_table_ - reference_to_series_table_; }

 private:
  uint64_t reference_to_symbols_table_ = 0;
  uint64_t reference_to_series_table_ = 0;
  uint64_t reference_to_label_indices_table_ = 0;
};

template <typename Reader, typename Callback>
void read_symbols(Reader& r, Callback&& func) {
  r.template skip_big_endian<uint32_t>();  // skip symbols table length
  const uint32_t number_of_symbols = r.template get_big_endian<uint32_t>();
  for (uint32_t i = 0; i < number_of_symbols; ++i) {
    const uint32_t symbol_len = r.template get_varint<uint32_t>();
    func(r.get_view(symbol_len));
  }
}

template <typename Reader>
void read_label_set(Reader& r, const SymbolsTable& st, PromPP::Prometheus::Block::LabelViewSet& ls) {
  const uint64_t labels = r.template get_varint<uint64_t>();
  for (uint64_t i = 0; i < labels; ++i) {
    const uint32_t ln_id = r.template get_varint<uint32_t>();
    const uint32_t lv_id = r.template get_varint<uint32_t>();
    ls.add({st[ln_id], st[lv_id]});
  }
}

template <typename Reader>
void read_chunks(Reader& r, const Timestamp& block_min_timestamp, PromPP::Prometheus::Block::Chunks& ch) {
  const uint64_t number_of_chunks = r.template get_varint<uint64_t>();

  Timestamp min_timestamp = r.template get_signed_varint<int64_t>();
  if (min_timestamp < block_min_timestamp) {
    throw std::runtime_error("R: Meaningful message supposed to be here!");
  }

  Timestamp max_timestamp = r.template get_varint<uint64_t>() + min_timestamp;

  uint64_t ref = r.template get_varint<uint64_t>();

  ch.add(min_timestamp, max_timestamp, ref);

  for (uint64_t i = 1; i < number_of_chunks; ++i) {
    min_timestamp = max_timestamp + r.template get_varint<uint64_t>();

    max_timestamp = min_timestamp + r.template get_varint<uint64_t>();

    ref = ref + r.template get_signed_varint<int64_t>();

    ch.add(min_timestamp, max_timestamp, ref);
  }
}

template <typename Reader>
void read_one_timeseries(Reader& r, const SymbolsTable& st, const Timestamp& block_min_timestamp, PromPP::Prometheus::Block::Series& s) {
  r.template skip_varint<uint32_t>();  // skip series length
  read_label_set(r, st, s.label_set());
  read_chunks(r, block_min_timestamp, s.chunks());
}

template <typename Reader, typename Callback>
void read_many_timeseries(Reader& r, const SymbolsTable& st, const Timestamp& block_min_timestamp, Callback&& func) {
  PromPP::Prometheus::Block::Series s;
  while (!r.eof()) {
    r.align(4);
    read_one_timeseries(r, st, block_min_timestamp, s);
    r.template skip_big_endian<uint32_t>();  // skip crc32 for series
    func(s);
    s.clear();
  }
}

}  // namespace PromPP::Prometheus::Block
