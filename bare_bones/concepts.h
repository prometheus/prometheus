#pragma once

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

}  // namespace BareBones::concepts