#pragma once

#include <string_view>

#include <lz4frame.h>

namespace BareBones::lz4 {

class CallResult {
 public:
  CallResult& operator=(LZ4F_errorCode_t result) noexcept {
    result_ = result;
    return *this;
  }

  [[nodiscard]] bool is_error() const noexcept { return LZ4F_isError(result_); }

  [[nodiscard]] size_t size() const noexcept { return result_; }
  [[nodiscard]] size_t error() const noexcept { return result_; }

  [[nodiscard]] std::string_view message() const noexcept { return LZ4F_getErrorName(result_); }

 private:
  LZ4F_errorCode_t result_{};
};

using CompressedBufferSize = uint32_t;

class CompressedBufferSizeHelper {
 public:
  [[nodiscard]] static CompressedBufferSize pack(CompressedBufferSize current_size, CompressedBufferSize actual_size) {
    auto compression_flag = current_size & kCompressFlag;
    return actual_size | compression_flag;
  }

  [[nodiscard]] static CompressedBufferSize unpack(CompressedBufferSize size) { return size & ~kCompressFlag; }

 private:
  static constexpr CompressedBufferSize kCompressFlag = 1 << 31;
};

}  // namespace BareBones::lz4
