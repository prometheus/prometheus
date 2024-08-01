#pragma once

#include <chrono>

namespace BareBones::concepts {

template <class T>
concept have_allocated_memory = requires(const T& t) {
  { t.allocated_memory() };
};

template <class T>
concept dereferenceable_have_allocated_memory = requires(const T& t) {
  { t->allocated_memory() };
};

template <class T>
concept have_capacity = requires(const T& t) {
  { t.capacity() };
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