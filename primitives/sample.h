#pragma once

#include <bit>

#include "bare_bones/preprocess.h"
#include "primitives.h"

namespace PromPP::Primitives {
class Sample {
 public:
  using timestamp_type = Timestamp;
  using value_type = double;

 private:
  timestamp_type timestamp_;
  value_type value_;

 public:
  [[nodiscard]] timestamp_type timestamp() const noexcept { return timestamp_; }
  timestamp_type& timestamp() noexcept { return timestamp_; }

  [[nodiscard]] value_type value() const noexcept { return value_; }
  value_type& value() noexcept { return value_; }

  template <size_t I>
  PROMPP_ALWAYS_INLINE const auto& get() const noexcept {
    if constexpr (I == 0)
      return timestamp_;
    if constexpr (I == 1)
      return value_;
    static_assert(I < 2);
  }

  template <size_t I>
  PROMPP_ALWAYS_INLINE auto& get() noexcept {
    if constexpr (I == 0)
      return timestamp_;
    if constexpr (I == 1)
      return value_;
    static_assert(I < 2);
  }

  template <class T>
  // TODO requires is_sample
  PROMPP_ALWAYS_INLINE Sample& operator=(const T& s) noexcept {
    timestamp_ = s.timestamp();
    value_ = s.value();
    return *this;
  }

  bool operator==(const Sample& other) const noexcept {
    return timestamp_ == other.timestamp_ && std::bit_cast<uint64_t>(value_) == std::bit_cast<uint64_t>(other.value_);
  }

  template <class T>
  // TODO requires is_sample
  PROMPP_ALWAYS_INLINE bool operator==(const T& s) const noexcept {
    return timestamp_ == s.timestamp() && std::bit_cast<uint64_t>(value_) == std::bit_cast<uint64_t>(s.value());
  }

  template <class T>
  // TODO requires is_sample
  PROMPP_ALWAYS_INLINE explicit Sample(const T& s) noexcept : timestamp_(s.timestamp()), value_(s.value()) {}

  PROMPP_ALWAYS_INLINE Sample(timestamp_type timestamp, value_type value) noexcept : timestamp_(timestamp), value_(value) {}

  PROMPP_ALWAYS_INLINE Sample() noexcept = default;
};

// ADL get function
template <std::size_t I>
constexpr auto& get(PromPP::Primitives::Sample& t) noexcept {
  return t.get<I>();
}

template <std::size_t I>
constexpr const auto& get(const PromPP::Primitives::Sample& t) noexcept {
  return t.get<I>();
}

}  // namespace PromPP::Primitives

// make Sample behave as std::tuple
namespace std {
template <>
struct tuple_size<PromPP::Primitives::Sample> : std::integral_constant<std::size_t, 2> {};

template <std::size_t I>
struct tuple_element<I, PromPP::Primitives::Sample> {
  using type = std::remove_cvref_t<decltype(std::declval<PromPP::Primitives::Sample>().get<I>())>;
};

}  // namespace std