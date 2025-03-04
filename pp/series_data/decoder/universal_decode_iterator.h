#pragma once

#include <variant>

#include "asc_integer_values_gorilla.h"
#include "constant.h"
#include "gorilla.h"
#include "two_double_constant.h"
#include "values_gorilla.h"

namespace series_data::decoder {

class UniversalDecodeIterator : public DecodeIteratorTypeTrait {
 public:
  template <class InPlaceType, class... Args>
  explicit UniversalDecodeIterator(InPlaceType in_place_type, Args&&... args) : iterator_(in_place_type, std::forward<Args>(args)...) {}

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const noexcept {
    return std::visit([](auto& iterator) PROMPP_LAMBDA_INLINE -> auto const& { return *iterator; }, iterator_);
  }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const noexcept {
    return std::visit([](auto& iterator) PROMPP_LAMBDA_INLINE -> auto const* { return iterator.operator->(); }, iterator_);
  }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel& sentinel) const noexcept {
    return std::visit([&sentinel](const auto& iterator) PROMPP_LAMBDA_INLINE { return iterator == sentinel; }, iterator_);
  }

  PROMPP_ALWAYS_INLINE UniversalDecodeIterator& operator++() noexcept {
    std::visit([](auto& iterator) PROMPP_LAMBDA_INLINE { ++iterator; }, iterator_);
    return *this;
  }

  PROMPP_ALWAYS_INLINE UniversalDecodeIterator operator++(int) noexcept {
    auto result = *this;
    ++*this;
    return result;
  }

 private:
  using IteratorVariant = std::variant<ConstantDecodeIterator,
                                       TwoDoubleConstantDecodeIterator,
                                       AscIntegerValuesGorillaDecodeIterator,
                                       ValuesGorillaDecodeIterator,
                                       GorillaDecodeIterator>;

  IteratorVariant iterator_;
};

}  // namespace series_data::decoder
