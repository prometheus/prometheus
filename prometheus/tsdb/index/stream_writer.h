#pragma once

#include <bit>
#include <string>

#include "absl/crc/crc32c.h"

#include "bare_bones/bit.h"
#include "bare_bones/concepts.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/varint.h"

namespace PromPP::Prometheus::tsdb::index {

template <class Writer>
concept WriterInterface = requires(Writer& writer, const Writer& const_writer, const void* memory) {
  { writer.write(memory, uint32_t()) };
  { writer.reserve(uint32_t()) };
  { const_writer.position() } -> std::same_as<uint32_t>;
  { const_writer.size() } -> std::same_as<uint32_t>;
};

template <class Stream>
class StreamWriterImpl {
 public:
  explicit StreamWriterImpl(Stream* stream = nullptr) : stream_(stream) {}

  PROMPP_ALWAYS_INLINE void reserve(uint32_t size) noexcept {
    if constexpr (BareBones::concepts::has_reserve<Stream>) {
      assert(stream_ != nullptr);

      stream_->reserve(size_ + size);
    }
  }

  PROMPP_ALWAYS_INLINE void set_stream(Stream* stream) noexcept {
    stream_ = stream;
    size_ = 0;
  }

  PROMPP_ALWAYS_INLINE void write(const void* memory, uint32_t size) noexcept {
    assert(stream_ != nullptr);

    stream_->write(reinterpret_cast<const char*>(memory), static_cast<std::streamsize>(size));
    position_ += size;
    size_ += size;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t position() const noexcept { return position_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return size_; }

 private:
  Stream* stream_;
  uint32_t position_{};
  uint32_t size_{};
};

class StringWriterImpl {
 public:
  PROMPP_ALWAYS_INLINE void reserve(uint32_t size) noexcept { buffer_.reserve(buffer_.size() + size); }

  PROMPP_ALWAYS_INLINE void write(const void* memory, uint32_t size) noexcept { buffer_.append(reinterpret_cast<const char*>(memory), size); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const std::string& buffer() const noexcept { return buffer_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t position() const noexcept { return buffer_.size(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return buffer_.size(); }

  PROMPP_ALWAYS_INLINE void free_memory() noexcept {
    buffer_.clear();
    buffer_.shrink_to_fit();
  }

  PROMPP_ALWAYS_INLINE void clear() noexcept { buffer_.clear(); }

 private:
  std::string buffer_;
};

struct Crc32Tag {};
struct NoCrc32Tag {};

template <WriterInterface WriterImpl>
class Writer {
 public:
  template <class... Args>
  explicit Writer(Args&&... args)
    requires(std::constructible_from<WriterImpl, Args...>)
      : writer_(std::forward<Args>(args)...) {}

  const WriterImpl& writer() const noexcept { return writer_; }
  WriterImpl& writer() noexcept { return writer_; }

  PROMPP_ALWAYS_INLINE void reserve(uint32_t size) noexcept { writer_.reserve(size); }

  template <class crc32 = Crc32Tag, class NumberType>
    requires(std::is_same_v<uint64_t, NumberType> || std::is_same_v<int64_t, NumberType>)
  PROMPP_ALWAYS_INLINE void write_varint(NumberType value) noexcept {
    uint8_t buffer[BareBones::Encoding::VarInt::kMaxVarIntLength];
    auto size = BareBones::Encoding::VarInt::write(buffer, value);
    writer_.write(buffer, size);

    if constexpr (std::is_same_v<crc32, Crc32Tag>) {
      extend_crc32(buffer, size);
    }
  }

  template <class crc32 = Crc32Tag, class NumberType>
  PROMPP_ALWAYS_INLINE void write_number(NumberType number) noexcept {
    writer_.write(&number, sizeof(number));

    if constexpr (std::is_same_v<crc32, Crc32Tag>) {
      extend_crc32(&number, sizeof(number));
    }
  }

  template <class crc32 = Crc32Tag>
  PROMPP_ALWAYS_INLINE void write_uint32(uint32_t value) noexcept {
    write_number<crc32>(BareBones::Bit::be(value));
  }

  template <class crc32 = Crc32Tag>
  PROMPP_ALWAYS_INLINE void write(uint8_t byte) noexcept {
    write_number<crc32>(byte);
  }

  template <class crc32 = Crc32Tag>
  PROMPP_ALWAYS_INLINE void write(std::string_view buffer) noexcept {
    writer_.write(buffer.data(), buffer.size());

    if constexpr (std::is_same_v<crc32, Crc32Tag>) {
      extend_crc32(buffer);
    }
  }

  PROMPP_ALWAYS_INLINE void write_crc32() noexcept { write_uint32(static_cast<uint32_t>(crc32_)); }
  PROMPP_ALWAYS_INLINE void reset_crc32() noexcept { crc32_ = absl::crc32c_t{0}; }

  void align_to(uint32_t size) noexcept {
    static constexpr uint64_t kZeroBytesPadding = 0ULL;

    if (auto rest = position() % size; rest != 0) {
      auto padding_bytes = size - rest;
      while (padding_bytes >= sizeof(kZeroBytesPadding)) {
        write_number<NoCrc32Tag>(kZeroBytesPadding);
        padding_bytes -= sizeof(kZeroBytesPadding);
      }

      writer_.write(&kZeroBytesPadding, padding_bytes);
    }
  }

  template <class PayloadWriter>
  void write_payload(uint32_t payload_size, PayloadWriter&& write_payload) noexcept {
    reset_crc32();
    reserve(sizeof(payload_size) + payload_size);

    write_payload();

    write_crc32();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t position() const noexcept { return writer_.position(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return writer_.size(); }

 private:
  WriterImpl writer_;
  absl::crc32c_t crc32_{0};

  PROMPP_ALWAYS_INLINE void extend_crc32(const void* memory, size_t size) noexcept { extend_crc32({reinterpret_cast<const char*>(memory), size}); }
  PROMPP_ALWAYS_INLINE void extend_crc32(std::string_view buffer) noexcept { crc32_ = absl::ExtendCrc32c(crc32_, buffer); }
};

template <class Stream>
using StreamWriter = Writer<StreamWriterImpl<Stream>>;
using StringWriter = Writer<StringWriterImpl>;

}  // namespace PromPP::Prometheus::tsdb::index