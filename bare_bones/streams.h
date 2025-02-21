#pragma once

#include <cassert>
#include <concepts>
#include <cstddef>
#include <cstdint>
#include <cstring>
#include <ostream>
#include <string_view>

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

class ShrinkedToFitOStringStream : public std::ostream {
 public:
  explicit ShrinkedToFitOStringStream() : std::ostream(&buffer_) {}

  [[nodiscard]] std::string_view view() const noexcept { return buffer_.view(); }

 private:
  class output_buffer final : public std::streambuf {
   public:
    ~output_buffer() override { std::free(memory_); }

    [[nodiscard]] std::string_view view() const noexcept { return {memory_, size_}; }

   private:
    char* memory_{};
    size_t size_{};

    int_type overflow(int_type ch) override {
      xsputn(reinterpret_cast<const char_type*>(&ch), 1);
      return ch;
    }

    std::streamsize xsputn(const char_type* s, std::streamsize count) override {
      memory_ = static_cast<char*>(std::realloc(memory_, size_ + count));
      assert(memory_ != nullptr);

      std::memcpy(memory_ + size_, s, count);
      size_ += count;

      return count;
    }
  };

  output_buffer buffer_;
};

}  // namespace BareBones
