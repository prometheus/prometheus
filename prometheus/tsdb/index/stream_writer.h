#pragma once

#include <bit>
#include <string>

#include "absl/crc/crc32c.h"

#include "bare_bones/bit.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/varint.h"

namespace PromPP::Prometheus::tsdb::index {

class StreamWriter {
 public:
  template <class NumberType>
  class NumberPlaceholder {
   public:
    explicit NumberPlaceholder(std::string& memory) : NumberPlaceholder(memory, memory.length()) {}
    NumberPlaceholder(std::string& memory, uint32_t offset) : memory_(&memory), offset_(offset) {}

    PROMPP_ALWAYS_INLINE NumberType get() const noexcept { return BareBones::Bit::be_to_native(*number()); }
    PROMPP_ALWAYS_INLINE void set(NumberType value) const noexcept { *number() = BareBones::Bit::be(value); }

   private:
    std::string* memory_;
    uint32_t offset_;

    [[nodiscard]] PROMPP_ALWAYS_INLINE NumberType* number() const noexcept { return reinterpret_cast<NumberType*>(memory_->data() + offset_); }
  };

  class Writer {
   public:
    explicit Writer(std::ostream* stream = nullptr) : stream_(stream) {}

    PROMPP_ALWAYS_INLINE void set_stream(std::ostream* stream) noexcept { stream_ = stream; }

    PROMPP_ALWAYS_INLINE void write_bytes(const void* memory, uint32_t size) noexcept {
      assert(stream_ != nullptr);

      stream_->write(reinterpret_cast<const char*>(memory), static_cast<std::streamsize>(size));
      position_ += size;
    }

    template <class Number>
    PROMPP_ALWAYS_INLINE void write_number(Number number) noexcept {
      write_bytes(&number, sizeof(number));
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t position() const noexcept { return position_; }

   private:
    std::ostream* stream_;
    size_t position_{};
  };

  explicit StreamWriter(std::ostream* stream = nullptr) : writer_(stream) {}

  template <class NumberType>
  [[nodiscard]] PROMPP_ALWAYS_INLINE static NumberPlaceholder<NumberType> write_number_placeholder(std::string& str) noexcept {
    NumberPlaceholder<NumberType> result{str};

    NumberType stub{};
    str.append(reinterpret_cast<const char*>(&stub), sizeof(stub));

    return result;
  }

  PROMPP_ALWAYS_INLINE static void write_uint32(uint32_t value, std::string& str) noexcept {
    value = BareBones::Bit::be(value);
    str.append(reinterpret_cast<const char*>(&value), sizeof(value));
  }

  PROMPP_ALWAYS_INLINE static void write_uvarint(uint64_t value, std::string& str) noexcept {
    uint8_t buffer[BareBones::Encoding::VarInt::kMaxVarIntLength];
    str.append(reinterpret_cast<const char*>(buffer), static_cast<std::streamsize>(BareBones::Encoding::VarInt::write(buffer, value)));
  }

  PROMPP_ALWAYS_INLINE static void write_varint(int64_t value, std::string& str) noexcept {
    uint8_t buffer[BareBones::Encoding::VarInt::kMaxVarIntLength];
    str.append(reinterpret_cast<const char*>(buffer), static_cast<std::streamsize>(BareBones::Encoding::VarInt::write(buffer, value)));
  }

  PROMPP_ALWAYS_INLINE static absl::crc32c_t crc32_of_varint(uint64_t value) noexcept {
    uint8_t buffer[BareBones::Encoding::VarInt::kMaxVarIntLength];
    return absl::ComputeCrc32c({reinterpret_cast<const char*>(buffer), BareBones::Encoding::VarInt::write(buffer, value)});
  }

  PROMPP_ALWAYS_INLINE void set_stream(std::ostream* stream) noexcept { writer_.set_stream(stream); }

  PROMPP_ALWAYS_INLINE void write_uvarint(uint64_t value) noexcept {
    uint8_t buffer[BareBones::Encoding::VarInt::kMaxVarIntLength];
    writer_.write_bytes(buffer, BareBones::Encoding::VarInt::write(buffer, value));
  }

  PROMPP_ALWAYS_INLINE void write_varint(int64_t value) noexcept {
    uint8_t buffer[BareBones::Encoding::VarInt::kMaxVarIntLength];
    writer_.write_bytes(buffer, BareBones::Encoding::VarInt::write(buffer, value));
  }

  PROMPP_ALWAYS_INLINE void write_uint32(uint32_t value) noexcept { writer_.write_number(BareBones::Bit::be(value)); }

  PROMPP_ALWAYS_INLINE void write_uint64(uint64_t value) noexcept { writer_.write_number(BareBones::Bit::be(value)); }

  PROMPP_ALWAYS_INLINE void write(uint8_t byte) noexcept { writer_.write_number(byte); }
  PROMPP_ALWAYS_INLINE void write(std::string_view buffer) noexcept { writer_.write_bytes(buffer.data(), buffer.size()); }

  PROMPP_ALWAYS_INLINE void compute_and_write_crc32(std::string_view buffer) noexcept { write_uint32(static_cast<uint32_t>(absl::ComputeCrc32c(buffer))); }

  PROMPP_ALWAYS_INLINE void align_to(uint32_t size) {
    static constexpr uint64_t kZeroBytesPadding = 0ULL;

    if (auto rest = position() % size; rest != 0) {
      auto padding_bytes = size - rest;
      while (padding_bytes >= sizeof(kZeroBytesPadding)) {
        writer_.write_number(kZeroBytesPadding);
        padding_bytes -= sizeof(kZeroBytesPadding);
      }

      writer_.write_bytes(&kZeroBytesPadding, padding_bytes);
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t position() const noexcept { return writer_.position(); }

 private:
  Writer writer_;
};

}  // namespace PromPP::Prometheus::tsdb::index