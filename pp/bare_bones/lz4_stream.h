/*
 * This software is partially based on the code from the lz4_stream repository.
 * Original repository by: laudrup
 * Original repository: https://github.com/laudrup/lz4_stream
 *
 * Modifications have been made to the original code to adapt it for use in this project.
 *
 * The original lz4_stream code is licensed under the BSD 3-Clause License, the terms of which are included below.
 *
 * Copyright (c) 2015, Kasper Laudrup <laudrup@stacktrace.dk>
 * All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions are
 * met:
 *
 * 1. Redistributions of source code must retain the above copyright
 * notice, this list of conditions and the following disclaimer.
 *
 * 2. Redistributions in binary form must reproduce the above copyright
 * notice, this list of conditions and the following disclaimer in the
 * documentation and/or other materials provided with the distribution.
 *
 * 3. Neither the name of the copyright holder nor the names of its
 * contributors may be used to endorse or promote products derived from
 * this software without specific prior written permission.
 *
 * THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
 * "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
 * LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
 * A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
 * HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
 * SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
 * LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
 * DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
 * THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
 * (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
 * OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 */

#pragma once

#include <array>
#include <cassert>
#include <cstring>
#include <streambuf>
#include <vector>

#include <lz4frame.h>
#include <lz4hc.h>

#include "preprocess.h"

namespace BareBones::LZ4Stream {
/**
 * @brief An output stream that will LZ4 compress the input data.
 *
 * An output stream that will wrap another output stream and LZ4
 * compress its input data to that stream.
 *
 */
template <size_t SrcBufSize = 256>
class basic_ostream final : public std::ostream {
 public:
  /**
   * @brief Constructs an LZ4 compression output stream
   *
   * @param stream The stream to write compressed data to
   */
  explicit basic_ostream(std::ostream* stream, bool write_header = true, int compression_level = LZ4HC_CLEVEL_DEFAULT)
      : std::ostream(&buffer_), buffer_(stream, !write_header, compression_level) {}

  /**
   * @brief Destroys the LZ4 output stream. Calls close() if not already called.
   */
  ~basic_ostream() override { close(); }

  /**
   * @brief Flushes and writes LZ4 footer data to the LZ4 output stream.
   *
   * After calling this function no more data should be written to the stream.
   */
  PROMPP_ALWAYS_INLINE void close() { buffer_.close(); }

  PROMPP_ALWAYS_INLINE void set_stream(std::ostream* stream) { buffer_.set_stream(stream); }

 private:
  class output_buffer final : public std::streambuf {
   public:
    output_buffer(const output_buffer&) = delete;
    output_buffer& operator=(const output_buffer&) = delete;

    output_buffer(std::ostream* stream, bool header_written, int compression_level) : stream_(stream), header_written_(header_written) {
      preferences_.compressionLevel = compression_level;

      // TODO: No need to recalculate the dest_buf_ size on each construction
      dest_buf_.reserve(LZ4F_compressBound(src_buf_.size(), &preferences_));

      char* base = &src_buf_.front();
      setp(base, base + src_buf_.size() - 1);

      if (const size_t ret = LZ4F_createCompressionContext(&ctx_, LZ4F_VERSION); LZ4F_isError(ret) != 0) {
        throw std::runtime_error(std::string("Failed to create LZ4 compression context: ") + LZ4F_getErrorName(ret));
      }
      write_header();
    }

    ~output_buffer() override { close(); }

    PROMPP_ALWAYS_INLINE void close() {
      if (closed_) {
        return;
      }

      if (stream_ != nullptr) {
        sync();
      }

      LZ4F_freeCompressionContext(ctx_);
      closed_ = true;
    }

    PROMPP_ALWAYS_INLINE void set_stream(std::ostream* stream) {
      stream_ = stream;
      write_header();
    }

   private:
    std::array<char, SrcBufSize> src_buf_;
    std::vector<char> dest_buf_;
    LZ4F_preferences_t preferences_ = LZ4F_INIT_PREFERENCES;
    std::ostream* stream_;
    LZ4F_compressionContext_t ctx_{};
    bool closed_{};
    bool is_initialized_{};
    bool header_written_{};

    int_type overflow(int_type ch) override {
      assert(pptr() <= epptr());

      *pptr() = static_cast<basic_ostream::char_type>(ch);
      pbump(1);
      compress_and_write();

      return ch;
    }

    int_type sync() override {
      compress_and_write();
      flush();
      return 0;
    }

    PROMPP_ALWAYS_INLINE void compress_and_write() {
      // TODO: Throw exception instead or set badbit
      assert(!closed_);
      assert(stream_ != nullptr);

      const int orig_size = static_cast<int>(pptr() - pbase());
      pbump(-orig_size);
      const size_t ret = LZ4F_compressUpdate(ctx_, &dest_buf_.front(), dest_buf_.capacity(), pbase(), orig_size, nullptr);
      if (LZ4F_isError(ret) != 0) {
        throw std::runtime_error(std::string("LZ4 compression failed: ") + LZ4F_getErrorName(ret));
      }
      stream_->write(&dest_buf_.front(), ret);
    }

    PROMPP_ALWAYS_INLINE void flush() {
      assert(!closed_);
      assert(stream_ != nullptr);

      const size_t ret = LZ4F_flush(ctx_, &dest_buf_.front(), dest_buf_.capacity(), nullptr);
      if (LZ4F_isError(ret) != 0) {
        throw std::runtime_error(std::string("LZ4 flush failed: ") + LZ4F_getErrorName(ret));
      }
      stream_->write(&dest_buf_.front(), ret);
    }

    PROMPP_ALWAYS_INLINE void write_header() {
      // TODO: Throw exception instead or set badbit
      assert(!closed_);

      if (stream_ == nullptr) {
        return;
      }

      if (!is_initialized_) {
        const size_t ret = LZ4F_compressBegin(ctx_, &dest_buf_.front(), dest_buf_.capacity(), &preferences_);
        if (LZ4F_isError(ret) != 0) {
          throw std::runtime_error(std::string("Failed to start LZ4 compression: ") + LZ4F_getErrorName(ret));
        }

        if (!header_written_) {
          stream_->write(&dest_buf_.front(), ret);
          header_written_ = true;
        }

        is_initialized_ = true;
      }
    }
  };

  output_buffer buffer_;
};

/**
 * @brief An input stream that will LZ4 decompress output data.
 *
 * An input stream that will wrap another input stream and LZ4
 * decompress its output data to that stream.
 *
 */
template <size_t DecompressedBufferSize = 256>
class basic_istream final : public std::istream {
 public:
  /**
   * @brief Constructs an LZ4 decompression input stream
   *
   * @param stream The stream to read LZ4 compressed data from
   */
  explicit basic_istream(std::istream* stream) : std::istream(&buffer_), buffer_(stream) {}

  PROMPP_ALWAYS_INLINE void set_stream(std::istream* stream) {
    buffer_.set_stream(stream);
    clear();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return buffer_.allocated_memory(); }

 private:
  class input_buffer final : public std::streambuf {
   public:
    explicit input_buffer(std::istream* stream) : stream_(stream) {
      if (const size_t ret = LZ4F_createDecompressionContext(&ctx_, LZ4F_VERSION); LZ4F_isError(ret) != 0) {
        throw std::runtime_error(std::string("Failed to create LZ4 decompression context: ") + LZ4F_getErrorName(ret));
      }
      set_pointers(0);
    }

    ~input_buffer() override { LZ4F_freeDecompressionContext(ctx_); }

    int_type underflow() override {
      assert(stream_ != nullptr);

      if (data_block_count_ == 0) [[unlikely]] {
        if (!read_first_data_block()) {
          return traits_type::eof();
        }
      }

      while (true) {
        if (const auto decompressed_size = decompress(); decompressed_size != 0) {
          set_pointers(decompressed_size);
          return traits_type::to_int_type(*gptr());
        }

        if (!read_data_block(0)) {
          return traits_type::eof();
        }
      }
    }

    input_buffer(const input_buffer&) = delete;
    input_buffer& operator=(const input_buffer&) = delete;

    PROMPP_ALWAYS_INLINE void set_stream(std::istream* stream) noexcept {
      stream_ = stream;
      set_pointers(0);
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return LZ4F_allocated_memory(ctx_); }

   private:
    std::array<char, DecompressedBufferSize> decompressed_buffer_;
    std::string src_buf_;
    std::string_view source_buffer_view_;
    std::array<char, LZ4F_HEADER_SIZE_MAX + LZ4F_BLOCK_HEADER_SIZE> data_block_header_;
    std::istream* stream_;
    size_t data_block_count_{};
    LZ4F_decompressionContext_t ctx_{};

    PROMPP_ALWAYS_INLINE void set_pointers(size_t decompressed_size) {
      setg(&decompressed_buffer_.front(), &decompressed_buffer_.front(), &decompressed_buffer_.front() + decompressed_size);
    }

    [[nodiscard]] size_t read_header() {
      stream_->read(&data_block_header_.front(), LZ4F_MIN_SIZE_TO_KNOW_HEADER_LENGTH);
      const auto header_size = LZ4F_headerSize(&data_block_header_.front(), stream_->gcount());
      if (LZ4F_isError(header_size)) {
        return 0;
      }

      const auto rest_of_header = header_size - LZ4F_MIN_SIZE_TO_KNOW_HEADER_LENGTH;
      stream_->read(&data_block_header_.front() + stream_->gcount(), rest_of_header);
      if (static_cast<size_t>(stream_->gcount()) != rest_of_header) {
        return 0;
      }

      return header_size;
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool read_first_data_block() {
      size_t header_size{};
      if (header_size = read_header(); header_size == 0) {
        throw std::runtime_error(std::string("Invalid LZ4 header: ") + LZ4F_getErrorName(header_size));
      }

      return read_data_block(header_size);
    }

    [[nodiscard]] bool read_data_block(size_t header_size) {
      const size_t data_block_size = read_data_block_size(header_size);
      if (data_block_size == 0) {
        return false;
      }

      ++data_block_count_;
      src_buf_.reserve(data_block_size);
      memcpy(&src_buf_.front(), &data_block_header_.front(), header_size);

      const auto read_size = data_block_size - header_size;
      stream_->read(&src_buf_.front() + header_size, read_size);
      if (static_cast<size_t>(stream_->gcount()) != read_size) {
        return false;
      }

      source_buffer_view_ = {&src_buf_.front(), header_size + stream_->gcount()};
      return true;
    }

    [[nodiscard]] uint32_t read_data_block_size(size_t& header_size) {
      stream_->read(&data_block_header_.front() + header_size, LZ4F_BLOCK_HEADER_SIZE);
      if (stream_->gcount() != LZ4F_BLOCK_HEADER_SIZE) {
        return 0;
      }

      auto data_block_size = *reinterpret_cast<uint32_t*>(&data_block_header_.front() + header_size);
      if (data_block_size == 0) [[unlikely]] {
        return 0;
      }

      data_block_size = unpack_data_block_size(data_block_size);
      header_size += LZ4F_BLOCK_HEADER_SIZE;
      return data_block_size + header_size;
    }

    PROMPP_ALWAYS_INLINE size_t decompress() {
      auto src_size = source_buffer_view_.size();
      auto decompressed_size = decompressed_buffer_.size();
      if (const size_t ret = LZ4F_decompress(ctx_, &decompressed_buffer_.front(), &decompressed_size, source_buffer_view_.data(), &src_size, nullptr);
          LZ4F_isError(ret) != 0) {
        throw std::runtime_error(std::string("LZ4 decompression failed: ") + LZ4F_getErrorName(ret));
      }

      source_buffer_view_.remove_prefix(src_size);
      return decompressed_size;
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE static uint32_t unpack_data_block_size(uint32_t size) noexcept {
      constexpr uint32_t kCompressFlag = 1 << 31;
      return size & ~kCompressFlag;
    }
  };

  input_buffer buffer_;
};

using ostream = basic_ostream<>;
using istream = basic_istream<>;

}  // namespace BareBones::LZ4Stream
