#pragma once

#include "preprocess.h"

namespace BareBones::compiler {

template <class Tp>
static PROMPP_ALWAYS_INLINE void do_not_optimize(const Tp& value) {
  __asm__ __volatile__("" ::"m"(value));
}

};  // namespace BareBones::compiler
