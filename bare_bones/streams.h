#pragma once

#include <concepts>
#include <cstddef>
#include <ios>

namespace BareBones {
template <typename T>
concept HiddenExceptions = requires(T x, std::ios_base::iostate e) {
  { x.exceptions() } -> std::convertible_to<std::ios_base::iostate>;
  x.exceptions(e);
};

template <typename T>
concept InputStream = HiddenExceptions<T> && requires(T x, char c, char* s, size_t n) {
  { x.get() } -> std::convertible_to<uint8_t>;
  { x.eof() } -> std::convertible_to<bool>;
  x.read(s, n);
};

template <typename T>
concept OutputStream = HiddenExceptions<T> && requires(T x, char c, char* s, size_t n) {
  x.put(c);
  x.write(s, n);
};
}  // namespace BareBones
