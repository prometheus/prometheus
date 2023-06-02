#pragma once

#include <array>
#include <cassert>
#include <cstdint>
#include <fstream>

#ifdef __x86_64__
#include <x86intrin.h>
#endif
#ifdef __ARM_FEATURE_CRC32
#include <arm_acle.h>
#endif

#include <scope_exit.h>

#include "bare_bones/streams.h"
#include "bare_bones/type_traits.h"

namespace BareBones {
class CRC32 {
  uint32_t crc_ = 0;

#if defined(__SSE4_2__)
  inline __attribute__((always_inline)) void compute(uint32_t data) noexcept { crc_ = _mm_crc32_u32(crc_, data); }

  inline __attribute__((always_inline)) void compute(uint64_t data) noexcept { crc_ = _mm_crc32_u64(crc_, data); }
#elif defined(__ARM_FEATURE_CRC32)
  inline __attribute__((always_inline)) void compute(uint32_t data) noexcept { crc_ = __crc32cw(crc_, data); }

  inline __attribute__((always_inline)) void compute(uint64_t data) noexcept { crc_ = __crc32cd(crc_, data); }
#else
  static constexpr auto generate_lut() noexcept {
    std::array<std::array<uint32_t, 256>, 8> lut{};

    for (uint32_t i = 0; i <= 0xFF; ++i) {
      uint32_t crc = i;
      for (uint8_t bit = 0; bit < 8; bit++) {
        if (crc & 1) {
          crc = (crc >> 1) ^ 0x82f63b78ul;
        } else {
          crc = (crc >> 1);
        }
      }
      lut[0][i] = crc;
    }

    for (size_t i = 0; i <= 0xFF; ++i) {
      lut[1][i] = (lut[0][i] >> 8) ^ lut[0][lut[0][i] & 0xFF];
      lut[2][i] = (lut[1][i] >> 8) ^ lut[0][lut[1][i] & 0xFF];
      lut[3][i] = (lut[2][i] >> 8) ^ lut[0][lut[2][i] & 0xFF];
      lut[4][i] = (lut[3][i] >> 8) ^ lut[0][lut[3][i] & 0xFF];
      lut[5][i] = (lut[4][i] >> 8) ^ lut[0][lut[4][i] & 0xFF];
      lut[6][i] = (lut[5][i] >> 8) ^ lut[0][lut[5][i] & 0xFF];
      lut[7][i] = (lut[6][i] >> 8) ^ lut[0][lut[6][i] & 0xFF];
    }

    return lut;
  }

  inline __attribute__((always_inline)) void compute(uint32_t data) noexcept {
    static constexpr auto lut = generate_lut();

    uint32_t one = data ^ crc_;
    crc_ = lut[0][one >> 24 & 0xFF] ^ lut[1][(one >> 16) & 0xFF] ^ lut[2][(one >> 8) & 0xFF] ^ lut[3][one & 0xFF];
  }

  inline __attribute__((always_inline)) void compute(uint64_t data) noexcept {
    static constexpr auto lut = generate_lut();

    const uint32_t data_one = *reinterpret_cast<uint32_t*>(&data) ^ crc_;
    const uint32_t data_two = *(reinterpret_cast<uint32_t*>(&data) + 1);

    crc_ = lut[0][data_two >> 24 & 0xFF] ^ lut[1][(data_two >> 16) & 0xFF] ^ lut[2][(data_two >> 8) & 0xFF] ^ lut[3][data_two & 0xFF] ^
           lut[4][data_one >> 24 & 0xFF] ^ lut[5][(data_one >> 16) & 0xFF] ^ lut[6][(data_one >> 8) & 0xFF] ^ lut[7][data_one & 0xFF];
  }
#endif
 public:
  bool operator==(const CRC32& o) const noexcept { return crc_ == o.crc_; }

  inline __attribute__((always_inline)) explicit operator uint32_t() const noexcept { return ~crc_; }

  inline __attribute__((always_inline)) void clear() noexcept { crc_ = 0; }

  inline __attribute__((always_inline)) friend CRC32& operator<<(CRC32& crc, uint32_t data) noexcept {
    crc.compute(data);
    return crc;
  }
  inline __attribute__((always_inline)) friend CRC32& operator<<(CRC32& crc, uint64_t data) noexcept {
    crc.compute(data);
    return crc;
  }
  inline __attribute__((always_inline)) friend CRC32& operator<<(CRC32& crc, int64_t data) noexcept {
    crc.compute(std::bit_cast<uint64_t>(data));
    return crc;
  }
  inline __attribute__((always_inline)) friend CRC32& operator<<(CRC32& crc, double data) noexcept {
    crc.compute(std::bit_cast<uint64_t>(data));
    return crc;
  }

  size_t save_size() const noexcept {
    // version is written and read by methods put() and get() and they write and read 1 byte
    return 1 + sizeof(crc_);
  }

  template <class OutputStream>
  friend OutputStream& operator<<(OutputStream& out, const CRC32& crc) {
    auto original_exceptions = out.exceptions();
    auto sg1 = std::experimental::scope_exit([&]() { out.exceptions(original_exceptions); });
    out.exceptions(std::ifstream::failbit | std::ifstream::badbit);

    // write version
    out.put(1);

    // write data
    auto data = static_cast<uint32_t>(crc);
    out.write(reinterpret_cast<const char*>(&data), sizeof(data));

    return out;
  }

  template <class InputStream>
  friend InputStream& operator>>(InputStream& in, CRC32& crc) {
    assert(crc.crc_ == 0);
    auto sg1 = std::experimental::scope_fail([&]() { crc.crc_ = 0; });

    // read version
    uint8_t version = in.get();

    // return successfully, if stream is empty
    if (in.eof())
      return in;

    // check version
    if (version != 1)
      throw std::runtime_error("G: Meaningful message supposed to be here!");

    auto original_exceptions = in.exceptions();
    auto sg2 = std::experimental::scope_exit([&]() { in.exceptions(original_exceptions); });
    in.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit);

    // read data
    in.read(reinterpret_cast<char*>(&crc.crc_), sizeof(crc.crc_));
    crc.crc_ = ~crc.crc_;

    return in;
  }
};

template <>
struct IsTriviallyReallocatable<CRC32> : std::true_type {};

template <>
struct IsZeroInitializable<CRC32> : std::true_type {};

template <>
struct IsTriviallyDestructible<CRC32> : std::true_type {};

}  // namespace BareBones
