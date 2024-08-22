#pragma once

#include <concepts>
#include <cstddef>
#include <cstdint>
#include <ios>
#include <istream>

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

class ExceptionsGuard {
 public:
  static constexpr auto kAllExceptions = std::ios_base::failbit | std::ios_base::badbit | std::ios_base::eofbit;

  explicit ExceptionsGuard(std::istream& stream, std::ios_base::iostate exceptions) : stream_(stream), source_exceptions_(stream_.exceptions()) {
    stream_.exceptions(exceptions);
  }

  ~ExceptionsGuard() { stream_.exceptions(source_exceptions_); }

 private:
  std::istream& stream_;
  std::ios_base::iostate source_exceptions_;
};

}  // namespace BareBones
