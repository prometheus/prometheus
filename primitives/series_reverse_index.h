#pragma once

#include <array>
#include <span>

#include "bare_bones/encoding.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/vector.h"

namespace PromPP::Primitives {

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

  [[nodiscard]] uint32_t allocated_memory() const noexcept {
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

 private:
  Type type_;
  uint32_t elements_count_{};
  alignas(alignof(SeriesIdSequence)) Array sequence_impl_buffer_;

  PROMPP_ALWAYS_INLINE void switch_to_sequence() {
    Array buffer_copy = sequence_impl_buffer_;

    new (&sequence_impl_buffer_) SeriesIdSequence();
    type_ = Type::kSequence;

    for (auto value_copy : buffer_copy) {
      const_cast<SeriesIdSequence&>(sequence()).push_back(value_copy);
    }
  }
};

}  // namespace PromPP::Primitives

namespace BareBones {

template <>
struct IsTriviallyReallocatable<PromPP::Primitives::CompactSeriesIdSequence> : std::true_type {};

}  // namespace BareBones

namespace PromPP::Primitives {

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

  [[nodiscard]] uint32_t allocated_memory() const noexcept {
    uint32_t allocated_memory = all_series_.allocated_memory();
    for (auto& series_sequence : series_by_value_) {
      allocated_memory += series_sequence.allocated_memory();
    }

    allocated_memory += series_by_value_.capacity() * sizeof(CompactSeriesIdSequence);
    return allocated_memory;
  }

 private:
  CompactSeriesIdSequence all_series_{CompactSeriesIdSequence::Type::kSequence};
  BareBones::Vector<CompactSeriesIdSequence> series_by_value_;
};

}  // namespace PromPP::Primitives

namespace BareBones {

template <>
struct IsTriviallyReallocatable<PromPP::Primitives::LabelReverseIndex> : std::true_type {};

}  // namespace BareBones

namespace PromPP::Primitives {

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

  [[nodiscard]] PROMPP_ALWAYS_INLINE const CompactSeriesIdSequence* get(uint32_t label_name_id) {
    return exists(label_name_id) ? labels_by_name_[label_name_id].get_all() : nullptr;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const CompactSeriesIdSequence* get(uint32_t label_name_id, uint32_t label_value_id) {
    return exists(label_name_id) ? labels_by_name_[label_name_id].get(label_value_id) : nullptr;
  }

  [[nodiscard]] uint32_t allocated_memory() const noexcept {
    uint32_t allocated_memory = 0;
    for (auto& label : labels_by_name_) {
      allocated_memory += label.allocated_memory();
    }

    allocated_memory += labels_by_name_.capacity() * sizeof(LabelReverseIndex);
    return allocated_memory;
  }

 private:
  BareBones::Vector<LabelReverseIndex> labels_by_name_;
};

}  // namespace PromPP::Primitives
