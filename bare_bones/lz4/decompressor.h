#pragma once

#include <concepts>
#include <string>

#include <lz4frame.h>
#include <lz4hc.h>

#include "helper.h"

#include <iostream>

namespace BareBones::lz4 {

struct DecompressResult {
  std::string_view data;
  size_t frame_size{};

  auto operator<=>(const DecompressResult&) const noexcept = default;
};

class AllocationSizeCalculator {
 public:
  struct Parameters {
    const size_t min_allocation_size;
    const double allocation_ratio;
    const double reallocation_ratio;
  };

 public:
  explicit AllocationSizeCalculator(const Parameters& parameters) : parameters_(parameters) {}

  [[nodiscard]] size_t get_allocation_size(size_t compressed_data_size) const noexcept {
    return std::max(calculate(compressed_data_size, parameters_.allocation_ratio), parameters_.min_allocation_size);
  }

  [[nodiscard]] size_t get_reallocation_size(size_t buffer_size) const noexcept { return calculate(buffer_size, parameters_.reallocation_ratio); }

 private:
  [[nodiscard]] size_t calculate(size_t value, double ratio) const noexcept { return static_cast<size_t>(value * ratio); }

 private:
  const Parameters parameters_;
};

template <class Buffer>
concept BufferConcept = requires(Buffer buffer) {
  { buffer.allocate(size_t()) } -> std::same_as<void>;
  { buffer.reallocate(size_t()) } -> std::same_as<void>;
  { buffer.data() } -> std::same_as<char*>;
  { buffer.size() } -> std::same_as<size_t>;
  { buffer.view(size_t()) } -> std::same_as<std::string_view>;
};

template <class Buffer>
  requires BufferConcept<Buffer>
class StreamDecompressor {
 public:
  StreamDecompressor(Buffer& buffer, const AllocationSizeCalculator::Parameters& allocation_parameters)
      : buffer_(buffer), size_calculator_(allocation_parameters) {}
  ~StreamDecompressor() { LZ4F_freeDecompressionContext(ctx_); }

  [[nodiscard]] bool is_initialized() const noexcept { return ctx_ != nullptr; }
  [[nodiscard]] const CallResult& lz4_call_result() const noexcept { return call_result_; }

  [[nodiscard]] bool initialize() {
    if (is_initialized()) {
      return true;
    }

    call_result_ = LZ4F_createDecompressionContext(&ctx_, LZ4F_VERSION);
    return !call_result_.is_error();
  }

  [[nodiscard]] DecompressResult decompress(std::string_view buffer) {
    DecompressResult result;

    if (!is_initialized()) {
      return result;
    }

    auto header_size = get_header_size(buffer);
    auto compressed_data_size = get_compressed_data_size(buffer, header_size);
    if (compressed_data_size == 0) {
      return result;
    }

    if (!shrink_to_frame_size(buffer, header_size + sizeof(CompressedBufferSize) + compressed_data_size)) {
      return result;
    }

    buffer_.allocate(size_calculator_.get_allocation_size(compressed_data_size));

    if (decompress_loop(buffer, result.data)) {
      result.frame_size = buffer.size();
    }

    return result;
  }

 private:
  [[nodiscard]] static bool shrink_to_frame_size(std::string_view& buffer, size_t actual_size) {
    if (buffer.size() >= actual_size) {
      buffer.remove_suffix(buffer.size() - actual_size);
      return true;
    }

    return false;
  }

  [[nodiscard]] size_t get_header_size(std::string_view buffer) {
    if (call_result_ = LZ4F_headerSize(buffer.data(), buffer.size()); call_result_.is_error()) {
      return 0;
    }

    return call_result_.size();
  }

  [[nodiscard]] static CompressedBufferSize get_compressed_data_size(std::string_view buffer, size_t header_size) {
    if (buffer.size() < sizeof(CompressedBufferSize) + header_size) {
      return 0;
    }

    auto packed_size = *reinterpret_cast<const CompressedBufferSize*>(buffer.data() + header_size);
    return CompressedBufferSizeHelper::unpack(packed_size);
  }

  [[nodiscard]] bool decompress_loop(std::string_view input_buffer, std::string_view& output_buffer) {
    size_t decompressed_bytes = 0;
    while (true) {
      auto output_buffer_ptr = buffer_.data() + decompressed_bytes;
      auto output_buffer_size = buffer_.size() - decompressed_bytes;
      auto input_buffer_size = input_buffer.size();
      if (call_result_ = LZ4F_decompress(ctx_, output_buffer_ptr, &output_buffer_size, input_buffer.data(), &input_buffer_size, nullptr);
          call_result_.is_error()) {
        return false;
      }

      input_buffer.remove_prefix(input_buffer_size);
      decompressed_bytes += output_buffer_size;

      if (output_buffer_size < buffer_.size() - decompressed_bytes) {
        output_buffer = buffer_.view(decompressed_bytes);
        return true;
      }

      buffer_.reallocate(size_calculator_.get_reallocation_size(buffer_.size()));
    }
  }

 private:
  Buffer& buffer_;
  AllocationSizeCalculator size_calculator_;
  LZ4F_decompressionContext_t ctx_{};
  CallResult call_result_;
};

}  // namespace BareBones::lz4
