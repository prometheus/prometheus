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
    NumberPlaceholder(std::string& memory, uint32_t offset) : memory_(memory), offset_(offset) {}

    PROMPP_ALWAYS_INLINE NumberType get() const noexcept { return BareBones::Bit::be_to_native(*number()); }
    PROMPP_ALWAYS_INLINE void set(NumberType value) const noexcept { *number() = BareBones::Bit::be(value); }

   private:
    std::string& memory_;
    uint32_t offset_;

    [[nodiscard]] PROMPP_ALWAYS_INLINE NumberType* number() const noexcept { return reinterpret_cast<NumberType*>(memory_.data() + offset_); }
  };

  explicit StreamWriter(std::ostream& stream) : stream_(stream) {}

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

  PROMPP_ALWAYS_INLINE void write_uvarint(uint64_t value) noexcept {
    uint8_t buffer[BareBones::Encoding::VarInt::kMaxVarIntLength];
    stream_.write(reinterpret_cast<const char*>(buffer), static_cast<std::streamsize>(BareBones::Encoding::VarInt::write(buffer, value)));
  }

  PROMPP_ALWAYS_INLINE void write_varint(int64_t value) noexcept {
    uint8_t buffer[BareBones::Encoding::VarInt::kMaxVarIntLength];
    stream_.write(reinterpret_cast<const char*>(buffer), static_cast<std::streamsize>(BareBones::Encoding::VarInt::write(buffer, value)));
  }

  PROMPP_ALWAYS_INLINE void write_uint32(uint32_t value) noexcept {
    value = BareBones::Bit::be(value);
    stream_.write(reinterpret_cast<const char*>(&value), sizeof(value));
  }

  PROMPP_ALWAYS_INLINE void write_uint64(uint64_t value) noexcept {
    value = BareBones::Bit::be(value);
    stream_.write(reinterpret_cast<const char*>(&value), sizeof(value));
  }

  PROMPP_ALWAYS_INLINE void write(uint8_t byte) noexcept { stream_ << byte; }
  PROMPP_ALWAYS_INLINE void write(std::string_view buffer) noexcept { stream_ << buffer; }

  PROMPP_ALWAYS_INLINE void compute_and_write_crc32(std::string_view buffer) noexcept { write_uint32(static_cast<uint32_t>(absl::ComputeCrc32c(buffer))); }

  PROMPP_ALWAYS_INLINE void align_to(uint32_t size) {
    static constexpr uint64_t kZeroBytesPadding = 0ULL;

    if (auto rest = position() % size; rest != 0) {
      auto padding_bytes = size - rest;
      while (padding_bytes >= sizeof(kZeroBytesPadding)) {
        stream_.write(reinterpret_cast<const char*>(&kZeroBytesPadding), sizeof(kZeroBytesPadding));
        padding_bytes -= sizeof(kZeroBytesPadding);
      }

      stream_.write(reinterpret_cast<const char*>(&kZeroBytesPadding), static_cast<ssize_t>(padding_bytes));
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t position() const noexcept { return stream_.tellp(); }

 private:
  std::ostream& stream_;
};

}  // namespace PromPP::Prometheus::tsdb::index