#pragma once

#include <array>
#include <span>

#include "bare_bones/encoding.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/vector.h"

namespace series_index {

static constexpr uint32_t kOptimalPreAllocationElementsCount = 8;

using SeriesIdSequence = BareBones::EncodedSequence<
    BareBones::Encoding::DeltaRLE<BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec0124, kOptimalPreAllocationElementsCount>>>;

class CompactSeriesIdSequence {
 public:
  enum class Type : uint32_t { kArray = 0, kSequence };

  static constexpr uint32_t kMaxElementsInArray = sizeof(SeriesIdSequence) / sizeof(SeriesIdSequence::value_type);
  using Array = std::array<SeriesIdSequence::value_type, kMaxElementsInArray>;
  using value_type = typename SeriesIdSequence::value_type;

  PROMPP_ALWAYS_INLINE explicit CompactSeriesIdSequence(Type type) : type_(type) {
    if (type_ == Type::kSequence) {
      new (&sequence_impl_buffer_) SeriesIdSequence();
    }
  }

  PROMPP_ALWAYS_INLINE ~CompactSeriesIdSequence() {
    if (type_ == Type::kSequence) {
      sequence().~SeriesIdSequence();
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Type type() const noexcept { return type_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t count() const noexcept { return elements_count_; }

  void push_back(uint32_t value) {
    if (type_ == Type::kArray) {
      if (elements_count_ < kMaxElementsInArray) {
        sequence_impl_buffer_[elements_count_] = value;
        ++elements_count_;
        return;
      }

      switch_to_sequence();
    }

    const_cast<SeriesIdSequence&>(sequence()).push_back(value);
    ++elements_count_;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    if (type_ == Type::kArray) {
      return 0;
    }

    return sequence().allocated_memory();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesIdSequence& sequence() const noexcept {
    assert(type_ == Type::kSequence);
    return *reinterpret_cast<const SeriesIdSequence*>(sequence_impl_buffer_.data());
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const SeriesIdSequence::value_type> array() const noexcept {
    assert(type_ == Type::kArray);
    return {reinterpret_cast<const SeriesIdSequence::value_type*>(sequence_impl_buffer_.data()), elements_count_};
  }

  template <class Processor>
  PROMPP_ALWAYS_INLINE auto process_series(Processor&& processor) const noexcept {
    if (type_ == Type::kArray) {
      return processor(array());
    } else {
      return processor(sequence());
    }
  }

 private:
  Type type_;
  uint32_t elements_count_{};
  alignas(alignof(SeriesIdSequence)) Array sequence_impl_buffer_;

  PROMPP_ALWAYS_INLINE void switch_to_sequence() {
    Array buffer_copy = sequence_impl_buffer_;

    new (&sequence_impl_buffer_) SeriesIdSequence();
    type_ = Type::kSequence;

    std::ranges::copy(buffer_copy, std::back_inserter(const_cast<SeriesIdSequence&>(sequence())));
  }
};

}  // namespace series_index

namespace BareBones {

template <>
struct IsTriviallyReallocatable<series_index::CompactSeriesIdSequence> : std::true_type {};

}  // namespace BareBones

namespace series_index {

class LabelReverseIndex {
 public:
  PROMPP_ALWAYS_INLINE void add(uint32_t label_value_id, uint32_t series_id) {
    if (exists(label_value_id)) {
      series_by_value_[label_value_id].push_back(series_id);
    } else {
      series_by_value_.emplace_back(CompactSeriesIdSequence::Type::kArray).push_back(series_id);
    }

    all_series_.push_back(series_id);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool exists(uint32_t label_value_id) const noexcept { return label_value_id < series_by_value_.size(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const CompactSeriesIdSequence* get(uint32_t label_value_id) const noexcept {
    return exists(label_value_id) ? &series_by_value_[label_value_id] : nullptr;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const CompactSeriesIdSequence* get_all() const noexcept { return &all_series_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return all_series_.allocated_memory() + series_by_value_.allocated_memory(); }

 private:
  CompactSeriesIdSequence all_series_{CompactSeriesIdSequence::Type::kSequence};
  BareBones::Vector<CompactSeriesIdSequence> series_by_value_;
};

}  // namespace series_index

namespace BareBones {

template <>
struct IsTriviallyReallocatable<series_index::LabelReverseIndex> : std::true_type {};

}  // namespace BareBones

namespace series_index {

class SeriesReverseIndex {
 public:
  template <class Label>
  PROMPP_ALWAYS_INLINE void add(const Label& label, uint32_t series_id) {
    if (exists(label.name_id())) {
      labels_by_name_[label.name_id()].add(label.value_id(), series_id);
    } else {
      labels_by_name_.emplace_back().add(label.value_id(), series_id);
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool exists(uint32_t label_name_id) const noexcept { return label_name_id < labels_by_name_.size(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const CompactSeriesIdSequence* get(uint32_t label_name_id) const {
    return exists(label_name_id) ? labels_by_name_[label_name_id].get_all() : nullptr;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const CompactSeriesIdSequence* get(uint32_t label_name_id, uint32_t label_value_id) const {
    return exists(label_name_id) ? labels_by_name_[label_name_id].get(label_value_id) : nullptr;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return labels_by_name_.allocated_memory(); }

 private:
  BareBones::Vector<LabelReverseIndex> labels_by_name_;
};

}  // namespace series_index
