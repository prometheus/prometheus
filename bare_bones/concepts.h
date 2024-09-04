#pragma once

#include <chrono>

namespace BareBones::concepts {

template <class T>
concept has_allocated_memory = requires(const T& t) {
  { t.allocated_memory() };
};

template <class T>
concept dereferenceable_has_allocated_memory = requires(const T& t) {
  { t->allocated_memory() };
};

template <class T>
concept has_capacity = requires(const T& t) {
  { t.capacity() };
};

template <class T>
concept has_reserve = requires(T& r) {
  { r.reserve(size_t{}) };
};

template <class Clock>
concept SystemClockInterface = requires(Clock& clock) {
  { typename Clock::time_point{} } -> std::same_as<std::chrono::system_clock::time_point>;
  { clock.now() };
};

template <class Clock>
concept SteadyClockInterface = requires(Clock& clock) {
  { typename Clock::time_point{} } -> std::same_as<std::chrono::steady_clock::time_point>;
  { clock.now() };
};

}  // namespace BareBones::concepts
