#pragma once

#include <string_view>

#define LZ4F_STATIC_LINKING_ONLY
#include <lz4frame.h>
#undef LZ4F_STATIC_LINKING_ONLY

#include "bare_bones/preprocess.h"

namespace BareBones::lz4stream {

class CallResult {
 public:
  ALWAYS_INLINE CallResult& operator=(LZ4F_errorCode_t result) noexcept {
    result_ = result;
    return *this;
  }

  [[nodiscard]] ALWAYS_INLINE bool is_error() const noexcept { return LZ4F_isError(result_); }

  [[nodiscard]] ALWAYS_INLINE size_t size() const noexcept { return result_; }
  [[nodiscard]] ALWAYS_INLINE LZ4F_errorCodes error() const noexcept { return LZ4F_getErrorCode(result_); }

  [[nodiscard]] ALWAYS_INLINE std::string_view message() const noexcept { return LZ4F_getErrorName(result_); }

 private:
  LZ4F_errorCode_t result_{};
};

using CompressedBufferSize = uint32_t;

class CompressedBufferSizeHelper {
 public:
  [[nodiscard]] ALWAYS_INLINE static CompressedBufferSize pack(CompressedBufferSize current_size, CompressedBufferSize actual_size) {
    auto compression_flag = current_size & kCompressFlag;
    return actual_size | compression_flag;
  }

  [[nodiscard]] ALWAYS_INLINE static CompressedBufferSize unpack(CompressedBufferSize size) { return size & ~kCompressFlag; }

 private:
  static constexpr CompressedBufferSize kCompressFlag = 1 << 31;
};

}  // namespace BareBones::lz4stream
