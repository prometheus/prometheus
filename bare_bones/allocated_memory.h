#pragma once

#include "concepts.h"

namespace BareBones::mem {
namespace concepts {
using BareBones::concepts::dereferenceable_has_allocated_memory;
using BareBones::concepts::has_allocated_memory;
}  // namespace concepts

template <class T>
[[nodiscard]] constexpr size_t allocated_memory(T&& value) {
  if constexpr (mem::concepts::has_allocated_memory<T>) {
    return value.allocated_memory();
  } else if constexpr (mem::concepts::dereferenceable_has_allocated_memory<T>) {
    return value->allocated_memory();
  } else {
    return 0;
  }
}

}  // namespace BareBones::mem