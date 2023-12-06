#pragma once

#include <string>

#include <lz4frame.h>
#include <lz4hc.h>

#include "bare_bones/preprocess.h"
#include "helper.h"

namespace BareBones::lz4chain {

class Compressor {
 public:
  ~Compressor() { LZ4F_freeCompressionContext(ctx_); }

  [[nodiscard]] ALWAYS_INLINE bool is_initialized() const noexcept { return ctx_ != nullptr; }
  [[nodiscard]] ALWAYS_INLINE const CallResult& lz4_call_result() const noexcept { return call_result_; }

  [[nodiscard]] bool initialize(int compression_level = LZ4HC_CLEVEL_DEFAULT) {
    if (is_initialized()) {
      return true;
    }

    if (call_result_ = LZ4F_createCompressionContext(&ctx_, LZ4F_VERSION); call_result_.is_error()) {
      return false;
    }

    create_preferences(compression_level);
    return true;
  }

  [[nodiscard]] std::string_view compress(std::string_view data_frame) {
    std::string_view result;

    if (!is_initialized()) {
      return result;
    }

    auto create_header = data_frame_count_ == 0;
    allocate_buffer(data_frame, create_header);

    if (result = compress_data_frame(data_frame, create_header); !call_result_.is_error()) {
      ++data_frame_count_;
    }

    return result;
  }

 private:
  static constexpr size_t kHeaderMaxSize = LZ4F_HEADER_SIZE_MAX;

 private:
  void ALWAYS_INLINE create_preferences(int compression_level) noexcept {
    preferences_.compressionLevel = compression_level;
    preferences_.autoFlush = 1;
  }

  void ALWAYS_INLINE allocate_buffer(std::string_view data_frame, bool create_header) {
    auto size = LZ4F_compressBound(data_frame.size(), &preferences_);

    if (create_header) {
      size += kHeaderMaxSize;
    }

    buffer_.reserve(size);
  }

  [[nodiscard]] std::string_view compress_data_frame(std::string_view data_frame, bool create_header) {
    std::string_view result;

    size_t header_size = 0;
    if (create_header) {
      if (call_result_ = LZ4F_compressBegin(ctx_, &buffer_[0], kHeaderMaxSize, &preferences_); call_result_.is_error()) {
        return result;
      }

      header_size = call_result_.size();
    }

    auto output_buffer = &buffer_[0] + header_size;
    auto output_buffer_size = buffer_.capacity() - header_size;
    if (call_result_ = LZ4F_compressUpdate(ctx_, output_buffer, output_buffer_size, data_frame.data(), data_frame.size(), nullptr); !call_result_.is_error()) {
      result = {&buffer_[0], header_size + call_result_.size()};
      patch_compressed_buffer_size(output_buffer, call_result_.size());
    }

    return result;
  }

  static void ALWAYS_INLINE patch_compressed_buffer_size(char* buffer, size_t compressed_frame_size) {
    if (compressed_frame_size >= sizeof(CompressedBufferSize)) {
      auto actual_size = static_cast<CompressedBufferSize>(compressed_frame_size - sizeof(CompressedBufferSize));
      auto* size_in_buffer = reinterpret_cast<CompressedBufferSize*>(buffer);
      *size_in_buffer = CompressedBufferSizeHelper::pack(*size_in_buffer, actual_size);
    }
  }

 private:
  LZ4F_preferences_t preferences_ = LZ4F_INIT_PREFERENCES;
  std::string buffer_;
  LZ4F_compressionContext_t ctx_{};
  CallResult call_result_;
  size_t data_frame_count_{};
};

}  // namespace BareBones::lz4chain
