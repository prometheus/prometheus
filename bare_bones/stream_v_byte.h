#pragma once

#include <fstream>
#include <utility>

#include <scope_exit.h>

#include "exception.h"
#include "streams.h"
#include "type_traits.h"
#include "vector.h"

namespace BareBones {
namespace StreamVByte {

inline __attribute__((always_inline)) uint32_t keys_size(uint32_t size) noexcept {
  return (size + 3) / 4;
}

class Codec1234 {
  static constexpr auto generate_key_to_length_lut() noexcept {
    std::array<uint8_t, 256> lut;

    for (uint8_t a = 0; a != 4; ++a) {
      for (uint8_t b = 0; b != 4; ++b) {
        for (uint8_t c = 0; c != 4; ++c) {
          for (uint8_t d = 0; d != 4; ++d) {
            // code 0 doesn't mean 0 byte, but we account for that when the lut is used
            lut[a | (b << 2) | (c << 4) | (d << 6)] = a + b + c + d;
          }
        }
      }
    }

    return lut;
  }

 protected:
  template <std::contiguous_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value
  inline __attribute__((always_inline)) static uint8_t encode_branchless_tresspassing(uint32_t val, IteratorType& d_i) noexcept {
    /**
     * ATTENTION! This method trespasses up to 3 bytes further after the last element, so you should
     * somehow account for that. One of the possible solution might be to reserve/resize for 3 extra
     * bytes after the end.
     */
    uint8_t antisize_in_bytes = std::countl_zero(val) >> 3;

    *reinterpret_cast<uint32_t*>(&(*d_i)) = val;

    static const uint8_t code_lut[5] = {
        3,  // val is 25-32 bit
        2,  // val is 17-24 bit
        1,  // val is 9-16 bit
        0,  // val is 1-8 bit
        0   // val == 0
    };

    uint8_t code = code_lut[antisize_in_bytes];

    d_i += 1 + code;

    return code;
  }

  template <std::contiguous_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value
  inline __attribute__((always_inline)) static uint8_t encode_simple(uint32_t val, IteratorType& d_i) noexcept {
    if (val < (1 << 8)) {  // 1 byte
      *d_i++ = static_cast<uint8_t>(val);
      return 0;
    } else if (val < (1 << 16)) {  // 2 bytes
      *reinterpret_cast<uint16_t*>(&(*d_i)) = static_cast<uint16_t>(val);
      d_i += 2;
      return 1;
    } else if (val < (1 << 24)) {  // 3 bytes
      *reinterpret_cast<uint16_t*>(&(*d_i)) = static_cast<uint16_t>(val & 0xFFFFu);
      *(d_i + 2) = static_cast<uint8_t>(val >> 16);
      d_i += 3;
      return 2;
    } else {  // 4 bytes
      *reinterpret_cast<uint32_t*>(&(*d_i)) = val;
      d_i += 4;
      return 3;
    }
  }

  template <std::output_iterator<uint8_t> IteratorType>
    requires(!std::contiguous_iterator<IteratorType>)
  inline __attribute__((always_inline)) static uint8_t encode_simple(uint32_t val, IteratorType& d_i) noexcept {
    if (val < (1 << 8)) {  // 1 byte
      *d_i++ = static_cast<uint8_t>(val);
      return 0;
    } else if (val < (1 << 16)) {  // 2 bytes
      *d_i++ = static_cast<uint8_t>(val & 0xFF);
      *d_i++ = static_cast<uint8_t>(val >> 8);
      return 1;
    } else if (val < (1 << 24)) {  // 3 bytes
      *d_i++ = static_cast<uint8_t>(val & 0xFF);
      *d_i++ = static_cast<uint8_t>((val >> 8) & 0xFF);
      *d_i++ = static_cast<uint8_t>(val >> 16);
      return 2;
    } else {  // 4 bytes
      *d_i++ = static_cast<uint8_t>(val & 0xFF);
      *d_i++ = static_cast<uint8_t>((val >> 8) & 0xFF);
      *d_i++ = static_cast<uint8_t>((val >> 16) & 0xFF);
      *d_i++ = static_cast<uint8_t>(val >> 24);
      return 3;
    }
  }

 public:
  using value_type = uint32_t;
  template <std::contiguous_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value
  inline __attribute__((always_inline)) static uint8_t encode(uint32_t val, IteratorType& d_i) noexcept {
    return encode_branchless_tresspassing(val, d_i);
  }

  template <std::output_iterator<uint8_t> IteratorType>
    requires(!std::contiguous_iterator<IteratorType>)
  inline __attribute__((always_inline)) static uint8_t encode(uint32_t val, IteratorType& d_i) noexcept {
    return encode_simple(val, d_i);
  }

  template <std::contiguous_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value
  inline __attribute__((always_inline)) static uint32_t decode(uint8_t code, const IteratorType& d_i) noexcept {
    /**
     * ATTENTION! This method trespasses up to 3 bytes further after the last element, so you should
     * somehow account for that. One of the possible solution might be to reserve/resize for 3 extra
     * bytes after the end.
     */
    static constexpr uint32_t mask_lut[4] = {0xFF, 0xFFFF, 0xFFFFFF, 0xFFFFFFFF};
    return *reinterpret_cast<const uint32_t*>(&(*d_i)) & mask_lut[code];
  }

  template <std::random_access_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value && (!std::contiguous_iterator<IteratorType>)
  inline __attribute__((always_inline)) static uint32_t decode(uint8_t code, const IteratorType& d_i) noexcept {
    if (code == 0) {
      return *d_i;
    } else if (code == 1) {
      return *d_i | (*(d_i + 1) << 8);
    } else if (code == 2) {
      return *d_i | (*(d_i + 1) << 8) | (*(d_i + 2) << 16);
    } else {
      return *d_i | (*(d_i + 1) << 8) | (*(d_i + 2) << 16) | (*(d_i + 3) << 24);
    }
  }

  inline __attribute__((always_inline)) static uint8_t code_to_length(uint8_t code) noexcept { return code + 1; }

  template <std::forward_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value
  inline __attribute__((always_inline)) static size_t decode_data_size(uint32_t size, IteratorType begin) noexcept {
    static constexpr auto lut = generate_key_to_length_lut();

    // code 0 doesn't mean 0 byte, but in lut it is, so we need to account for that
    size_t res = size;
    for (auto k_i = begin; k_i != begin + keys_size(size); ++k_i) {
      res += lut[*k_i];
    }

    return res;
  }
};

class Codec1234Mostly1 : public Codec1234 {
 public:
  template <std::output_iterator<uint8_t> IteratorType>
  inline __attribute__((always_inline)) static uint8_t encode(uint32_t val, IteratorType& d_i) noexcept {
    return encode_simple(val, d_i);
  }
};

class Codec0124 {
  static constexpr uint8_t CODE_TO_LENGTH_LUT[4] = {0, 1, 2, 4};
  static constexpr auto generate_key_to_length_lut() noexcept {
    std::array<uint8_t, 256> lut;

    for (uint8_t a = 0; a != 4; ++a) {
      for (uint8_t b = 0; b != 4; ++b) {
        for (uint8_t c = 0; c != 4; ++c) {
          for (uint8_t d = 0; d != 4; ++d) {
            lut[a | (b << 2) | (c << 4) | (d << 6)] = CODE_TO_LENGTH_LUT[a] + CODE_TO_LENGTH_LUT[b] + CODE_TO_LENGTH_LUT[c] + CODE_TO_LENGTH_LUT[d];
          }
        }
      }
    }

    return lut;
  }

 protected:
  template <std::contiguous_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value
  inline __attribute__((always_inline)) static uint8_t encode_branchless_tresspassing(uint32_t val, IteratorType& d_i) noexcept {
    /**
     * ATTENTION! This method trespasses up to 4 bytes further after the last element, so you should
     * somehow account for that. One of the possible solution might be to reserve/resize for 3 extra
     * bytes after the end.
     */
    uint8_t antisize_in_bytes = std::countl_zero(val) >> 3;

    *reinterpret_cast<uint32_t*>(&(*d_i)) = val;

    static const uint8_t code_lut[5] = {
        3,  // val is 25-32 bit
        3,  // val is 17-24 bit
        2,  // val is 9-16 bit
        1,  // val is 1-8 bit
        0   // val == 0
    };

    uint8_t code = code_lut[antisize_in_bytes];

    d_i += code + static_cast<bool>(code == 3);

    return code;
  }

  template <std::contiguous_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value
  inline __attribute__((always_inline)) static uint8_t encode_simple(uint32_t val, IteratorType& d_i) noexcept {
    if (val == 0) {  // 0 byte
      return 0;
    } else if (val < (1 << 8)) {  // 1 byte
      *d_i++ = static_cast<uint8_t>(val);
      return 1;
    } else if (val < (1 << 16)) {  // 2 bytes
      *reinterpret_cast<uint16_t*>(&(*d_i)) = static_cast<uint16_t>(val);
      d_i += 2;
      return 2;
    } else {  // 4 bytes
      *reinterpret_cast<uint32_t*>(&(*d_i)) = val;
      d_i += 4;
      return 3;
    }
  }

  template <std::output_iterator<uint8_t> IteratorType>
    requires(!std::contiguous_iterator<IteratorType>)
  inline __attribute__((always_inline)) static uint8_t encode_simple(uint32_t val, IteratorType& d_i) noexcept {
    if (val == 0) {  // 0 byte
      return 0;
    } else if (val < (1 << 8)) {  // 1 byte
      *d_i++ = static_cast<uint8_t>(val);
      return 1;
    } else if (val < (1 << 16)) {  // 2 bytes
      *d_i++ = static_cast<uint8_t>(val & 0xFF);
      *d_i++ = static_cast<uint8_t>(val >> 8);
      return 2;
    } else {  // 4 bytes
      *d_i++ = static_cast<uint8_t>(val & 0xFF);
      *d_i++ = static_cast<uint8_t>((val >> 8) & 0xFF);
      *d_i++ = static_cast<uint8_t>((val >> 16) & 0xFF);
      *d_i++ = static_cast<uint8_t>(val >> 24);
      return 3;
    }
  }

 public:
  using value_type = uint32_t;
  template <std::contiguous_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value
  inline __attribute__((always_inline)) static uint8_t encode(uint32_t val, IteratorType& d_i) noexcept {
    return encode_branchless_tresspassing(val, d_i);
  }

  template <std::output_iterator<uint8_t> IteratorType>
    requires(!std::contiguous_iterator<IteratorType>)
  inline __attribute__((always_inline)) static uint8_t encode(uint32_t val, IteratorType& d_i) noexcept {
    return encode_simple(val, d_i);
  }

  template <std::contiguous_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value
  inline __attribute__((always_inline)) static uint32_t decode(uint8_t code, const IteratorType& d_i) noexcept {
    /*
     * ATTENTION! This method trespasses up to 4 bytes further after the last element, so you should
     * somehow account for that. One of the possible solution might be to reserve/resize for 4 extra
     * bytes after the end.
     */
    static const uint32_t mask_lut[4] = {0, 0xFF, 0xFFFF, 0xFFFFFFFF};
    return *reinterpret_cast<const uint32_t*>(&(*d_i)) & mask_lut[code];
  }

  template <std::random_access_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value && (!std::contiguous_iterator<IteratorType>)
  inline __attribute__((always_inline)) static uint32_t decode(uint8_t code, const IteratorType& d_i) noexcept {
    if (code == 0) {
      return 0;
    } else if (code == 1) {
      return *d_i;
    } else if (code == 2) {
      return *d_i | (*(d_i + 1) << 8);
    } else {
      return *d_i | (*(d_i + 1) << 8) | (*(d_i + 2) << 16) | (*(d_i + 3) << 24);
    }
  }

  inline __attribute__((always_inline)) static constexpr uint8_t code_to_length(uint8_t code) noexcept { return CODE_TO_LENGTH_LUT[code]; }

  template <std::forward_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value
  inline __attribute__((always_inline)) static size_t decode_data_size(uint32_t size, IteratorType begin) noexcept {
    static constexpr auto lut = generate_key_to_length_lut();

    size_t res = 0;
    for (auto k_i = begin; k_i != begin + keys_size(size); ++k_i) {
      res += lut[*k_i];
    }

    return res;
  }
};

class Codec0124Frequent0 : public Codec0124 {
 public:
  template <std::output_iterator<uint8_t> IteratorType>
  inline __attribute__((always_inline)) static uint8_t encode(uint32_t val, IteratorType& d_i) noexcept {
    return encode_simple(val, d_i);
  }
};

class Codec1238 {
  static constexpr uint8_t CODE_TO_LENGTH_LUT[4] = {0, 1, 2, 7};
  static constexpr auto generate_key_to_length_lut() noexcept {
    std::array<uint8_t, 256> lut;

    for (uint8_t a = 0; a != 4; ++a) {
      for (uint8_t b = 0; b != 4; ++b) {
        for (uint8_t c = 0; c != 4; ++c) {
          for (uint8_t d = 0; d != 4; ++d) {
            // code 0 doesn't mean 0 byte, but in lut it is, so we need to account for that
            lut[a | (b << 2) | (c << 4) | (d << 6)] = CODE_TO_LENGTH_LUT[a] + CODE_TO_LENGTH_LUT[b] + CODE_TO_LENGTH_LUT[c] + CODE_TO_LENGTH_LUT[d];
          }
        }
      }
    }

    return lut;
  }

 protected:
  template <std::contiguous_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value
  inline __attribute__((always_inline)) static uint8_t encode_branchless_tresspassing(uint64_t val, IteratorType& d_i) noexcept {
    /**
     * ATTENTION! This method trespasses up to 7 bytes further after the last element, so you should
     * somehow account for that. One of the possible solution might be to reserve/resize for 7 extra
     * bytes after the end.
     */
    uint8_t antisize_in_bytes = std::countl_zero(val) >> 3;

    *reinterpret_cast<uint64_t*>(&(*d_i)) = val;

    static const uint8_t code_lut[9] = {
        3,  // val is 57-64 bit
        3,  // val is 49-56 bit
        3,  // val is 41-48 bit
        3,  // val is 33-40 bit
        3,  // val is 25-32 bit
        2,  // val is 17-24 bit
        1,  // val is 9-16 bit
        0,  // val is 1-8 bit
        0   // val == 0
    };

    uint8_t code = code_lut[antisize_in_bytes];

    d_i += 1 + code + (code == 3) * 4;

    return code;
  }

  template <std::contiguous_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value
  inline __attribute__((always_inline)) static uint8_t encode_simple(uint64_t val, IteratorType& d_i) noexcept {
    if (val < (1 << 8)) {  // 1 byte
      *d_i++ = static_cast<uint8_t>(val);
      return 0;
    } else if (val < (1 << 16)) {  // 2 bytes
      *reinterpret_cast<uint16_t*>(&(*d_i)) = static_cast<uint16_t>(val);
      d_i += 2;
      return 1;
    } else if (val < (1 << 24)) {  // 3 bytes
      *reinterpret_cast<uint16_t*>(&(*d_i)) = static_cast<uint16_t>(val & 0xFFFFu);
      *(d_i + 2) = static_cast<uint8_t>(val >> 16);
      d_i += 3;
      return 2;
    } else {  // 8 bytes
      *reinterpret_cast<uint64_t*>(&(*d_i)) = val;
      d_i += 8;
      return 3;
    }
  }

  template <std::output_iterator<uint8_t> IteratorType>
    requires(!std::contiguous_iterator<IteratorType>)
  inline __attribute__((always_inline)) static uint8_t encode_simple(uint64_t val, IteratorType& d_i) noexcept {
    if (val < (1 << 8)) {  // 1 byte
      *d_i++ = static_cast<uint8_t>(val);
      return 0;
    } else if (val < (1 << 16)) {  // 2 bytes
      *d_i++ = static_cast<uint8_t>(val & 0xFF);
      *d_i++ = static_cast<uint8_t>(val >> 8);
      return 1;
    } else if (val < (1 << 24)) {  // 3 bytes
      *d_i++ = static_cast<uint8_t>(val & 0xFF);
      *d_i++ = static_cast<uint8_t>((val >> 8) & 0xFF);
      *d_i++ = static_cast<uint8_t>(val >> 16);
      return 2;
    } else {  // 8 bytes
      *d_i++ = static_cast<uint8_t>(val & 0xFF);
      *d_i++ = static_cast<uint8_t>((val >> 8) & 0xFF);
      *d_i++ = static_cast<uint8_t>((val >> 16) & 0xFF);
      *d_i++ = static_cast<uint8_t>((val >> 24) & 0xFF);
      *d_i++ = static_cast<uint8_t>((val >> 32) & 0xFF);
      *d_i++ = static_cast<uint8_t>((val >> 40) & 0xFF);
      *d_i++ = static_cast<uint8_t>((val >> 48) & 0xFF);
      *d_i++ = static_cast<uint8_t>((val >> 56));
      return 3;
    }
  }

 public:
  using value_type = uint64_t;

  template <std::contiguous_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value
  inline __attribute__((always_inline)) static uint8_t encode(uint64_t val, IteratorType& d_i) noexcept {
    return encode_branchless_tresspassing(val, d_i);
  }

  template <std::output_iterator<uint8_t> IteratorType>
    requires(!std::contiguous_iterator<IteratorType>)
  inline __attribute__((always_inline)) static uint8_t encode(uint64_t val, IteratorType& d_i) noexcept {
    return encode_simple(val, d_i);
  }

  template <std::contiguous_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value
  inline __attribute__((always_inline)) static uint64_t decode(uint8_t code, const IteratorType& d_i) noexcept {
    /**
     * ATTENTION! This method trespasses up to 7 bytes further after the last element, so you should
     * somehow account for that. One of the possible solution might be to reserve/resize for 7 extra
     * bytes after the end.
     */

    static constexpr uint64_t mask_lut[4] = {0xFF, 0xFFFF, 0xFFFFFF, 0xFFFFFFFFFFFFFFFF};
    return *reinterpret_cast<const uint64_t*>(&(*d_i)) & mask_lut[code];
  }

  template <std::random_access_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value && (!std::contiguous_iterator<IteratorType>)
  inline __attribute__((always_inline)) static uint64_t decode(uint8_t code, const IteratorType& d_i) noexcept {
    if (code == 0) {
      return *d_i;
    } else if (code == 1) {
      return *d_i | (*(d_i + 1) << 8);
    } else if (code == 2) {
      return *d_i | (*(d_i + 1) << 8) | (*(d_i + 2) << 16);
    } else {
      return (uint64_t)*d_i | ((uint64_t)(*(d_i + 1)) << 8) | ((uint64_t)(*(d_i + 2)) << 16) | ((uint64_t)(*(d_i + 3)) << 24) | ((uint64_t)(*(d_i + 4)) << 32) |
             ((uint64_t)(*(d_i + 5)) << 40) | ((uint64_t)(*(d_i + 6)) << 48) | ((uint64_t)(*(d_i + 7)) << 56);
    }
  }

  inline __attribute__((always_inline)) static uint8_t code_to_length(uint8_t code) noexcept { return code + 1 + (code == 3) * 4; }

  template <std::forward_iterator IteratorType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value
  inline __attribute__((always_inline)) static size_t decode_data_size(uint32_t size, IteratorType begin) noexcept {
    static constexpr auto lut = generate_key_to_length_lut();

    // code 0 doesn't mean 0 byte, but in lut it is, so we need to account for that
    size_t res = size;
    for (auto k_i = begin; k_i != begin + keys_size(size); ++k_i) {
      res += lut[*k_i];
    }

    return res;
  }
};

class Codec1238Mostly1 : public Codec1238 {
 public:
  template <std::output_iterator<uint8_t> IteratorType>
  inline __attribute__((always_inline)) static uint8_t encode(uint64_t val, IteratorType& d_i) noexcept {
    return encode_simple(val, d_i);
  }
};

template <class Codec, std::forward_iterator IteratorType>
  requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, uint8_t>::value
size_t decode_data_size(uint32_t size, IteratorType begin) noexcept {
  return Codec::decode_data_size(size, begin);
}

template <class Codec, std::forward_iterator KInnerIteratorType, std::output_iterator<uint8_t> DInnerIteratorType>
  requires std::is_same<typename std::iterator_traits<KInnerIteratorType>::value_type, uint8_t>::value
class EncodeIterator {
  KInnerIteratorType k_i_;
  DInnerIteratorType d_i_;

  uint8_t shift_ = 0;  // cycles 0, 2, 4, 6, 0, 2, 4, 6, ...

 public:
  using value_type = typename Codec::value_type;

 private:
  inline __attribute__((always_inline)) void encode(value_type val) noexcept {
    if (shift_ == 8) {
      shift_ = 0;
      ++k_i_;
      *k_i_ = 0;
    }

    auto code = Codec::encode(val, d_i_);
    *k_i_ |= code << shift_;
    shift_ += 2;
  }

 public:
  using iterator_category = std::output_iterator_tag;
  using difference_type = std::ptrdiff_t;

  inline __attribute__((always_inline)) explicit EncodeIterator(KInnerIteratorType k_i = {}, DInnerIteratorType d_i = {}) noexcept : k_i_(k_i), d_i_(d_i) {
    *k_i_ = 0;
  }

  inline __attribute__((always_inline)) EncodeIterator& operator++() noexcept { return *this; }
  inline __attribute__((always_inline)) EncodeIterator& operator++(int) noexcept { return *this; }
  inline __attribute__((always_inline)) EncodeIterator& operator*() noexcept { return *this; }
  inline __attribute__((always_inline)) EncodeIterator& operator=(value_type val) noexcept {
    encode(val);
    return *this;
  }
};

class DecodeIteratorSentinel {};

template <class Codec, std::random_access_iterator InnerIteratorType>
  requires std::is_same<typename std::iterator_traits<InnerIteratorType>::value_type, uint8_t>::value
class DecodeIterator {
  using inner_iterator_type = InnerIteratorType;

  uint32_t n_;

  inner_iterator_type k_i_;
  inner_iterator_type d_i_;
  uint8_t shift_ = 0;  // cycles 0, 2, 4, 6, 0, 2, 4, 6, ...
  uint8_t code_;

 public:
  using iterator_category = std::input_iterator_tag;
  using value_type = typename Codec::value_type;
  using difference_type = std::ptrdiff_t;

  inline __attribute__((always_inline)) explicit DecodeIterator(inner_iterator_type k_i = inner_iterator_type(),
                                                                inner_iterator_type d_i = inner_iterator_type(),
                                                                uint32_t size = 0) noexcept
      : n_(size), k_i_(k_i), d_i_(d_i) {
    code_ = n_ == 0 ? 0 : *k_i_;
    code_ >>= shift_;
    code_ &= 0x03;
  }
  inline __attribute__((always_inline)) DecodeIterator(inner_iterator_type i, uint32_t size) noexcept : DecodeIterator(i, i + keys_size(size), size) {}

  inline __attribute__((always_inline)) DecodeIterator& operator++() noexcept {
    assert(n_ != 0);

    shift_ += 2;
    --n_;

    k_i_ += (shift_ == 8);
    shift_ &= 0x07;

    d_i_ += Codec::code_to_length(code_);

    code_ = (*k_i_ >> shift_) & 0x3;

    return *this;
  }

  inline __attribute__((always_inline)) DecodeIterator operator++(int) noexcept {
    DecodeIterator retval = *this;
    ++(*this);
    return retval;
  }
  inline __attribute__((always_inline)) bool operator==(const DecodeIterator& other) const noexcept { return n_ == other.n_; }
  inline __attribute__((always_inline)) bool operator==(const DecodeIteratorSentinel& other) const noexcept { return !n_; }

  inline __attribute__((always_inline)) value_type operator*() const noexcept { return Codec::decode(code_, d_i_); }
};

template <class Codec>
class Sequence {
  uint32_t size_ = 0;

  Memory<uint8_t> keys_;
  Memory<uint8_t> data_;

  Memory<uint8_t>::iterator k_i_ = nullptr;
  Memory<uint8_t>::iterator d_i_ = nullptr;

 public:
  using value_type = typename Codec::value_type;

 private:
  inline __attribute__((always_inline)) uint32_t next_size(uint32_t size) const noexcept {
    if (size == std::numeric_limits<uint32_t>::max())
      std::abort();

    // ceil up to next 1024
    return (size + 1024) & 0xFFFFFC00u;
  }

  inline __attribute__((always_inline)) void reserve_keys(uint32_t size) noexcept {
    assert(size >= size_);

    // grow to fit new size
    keys_.grow_to_fit_at_least_and_fill_with_zeros(size >> 2);
    k_i_ = keys_ + (size_ >> 2);
  }

  inline __attribute__((always_inline)) void reserve_data(uint32_t size, uint32_t actual_data_size) noexcept {
    assert(size >= size_);

    // grow to fit actual size plus new elements in worst case (sizeof(value_type) per element)
    //  IMPORTANT!r!! This asures, that codec can trespass for up to sizeof(value_type)! See codec encode
    //  and decode for more detail
    data_.grow_to_fit_at_least(actual_data_size + ((size - size_) * sizeof(value_type)));
    d_i_ = data_ + actual_data_size;
  }

  inline __attribute__((always_inline)) void reserve_for_next_1024() noexcept {
    const uint32_t new_size = next_size(size_);
    reserve_keys(new_size);
    reserve_data(new_size, d_i_ - data_);
  }

 public:
  using iterator = DecodeIterator<Codec, Vector<const uint8_t>::iterator>;
  using const_iterator = DecodeIterator<Codec, Vector<const uint8_t>::iterator>;
  using sentinel = DecodeIteratorSentinel;

  inline __attribute__((always_inline)) Sequence() noexcept = default;
  Sequence(const Sequence&) = delete;
  Sequence& operator=(const Sequence&) = delete;
  inline __attribute__((always_inline)) Sequence(Sequence&&) noexcept = default;
  inline __attribute__((always_inline)) Sequence& operator=(Sequence&&) noexcept = default;

  inline __attribute__((always_inline)) void push_back(value_type val) noexcept {
    // if size_ divides by 1024 without reminders
    if (!(size_ & 0x3FF))
      reserve_for_next_1024();

    assert(static_cast<size_t>(d_i_ - data_ + 4) <= data_.size());
    assert(d_i_ >= data_);
    assert(static_cast<size_t>(k_i_ - keys_) < data_.size());
    assert(k_i_ >= keys_);

    auto code = Codec::encode(val, d_i_);
    *k_i_ |= code << ((size_ & 0x03) << 1);

    ++size_;

    // move key if size_ % 4 == 0
    k_i_ += !(size_ & 0x03);
  }

  inline __attribute__((always_inline)) void clear() noexcept {
    if (size_ != 0) {
      size_ = 0;
      std::memset(keys_, 0, k_i_ - keys_ + 1);
      k_i_ = keys_;
      d_i_ = data_;
    }
  }

  // TODO: shrink_to_fit

  inline __attribute__((always_inline)) size_t size() const noexcept { return size_; }

  inline __attribute__((always_inline)) bool empty() const noexcept { return !size_; }

  inline __attribute__((always_inline)) auto begin() const noexcept { return const_iterator(keys_, data_, size_); }

  inline __attribute__((always_inline)) auto end() const noexcept { return sentinel(); }

  inline __attribute__((always_inline)) size_t save_size() const noexcept {
    // version is written and read by methods put() and get() and they write and read 1 byte
    return 1 + sizeof(uint32_t) + keys_size(size_) + (d_i_ - data_);
  }

  template <OutputStream S>
  friend S& operator<<(S& out, const Sequence& seq) {
    auto original_exceptions = out.exceptions();
    auto sg1 = std::experimental::scope_exit([&]() { out.exceptions(original_exceptions); });
    out.exceptions(std::ifstream::failbit | std::ifstream::badbit);

    // write version
    out.put(1);

    // write size
    out.write(reinterpret_cast<const char*>(&seq.size_), sizeof(seq.size_));

    // if there are no items to write, we finish here
    if (!seq.size_) {
      return out;
    }

    // write keys
    out.write(reinterpret_cast<const char*>(static_cast<const uint8_t*>(seq.keys_)), keys_size(seq.size_));

    // write data
    out.write(reinterpret_cast<const char*>(static_cast<const uint8_t*>(seq.data_)), seq.d_i_ - seq.data_);

    return out;
  }

  template <InputStream S>
  friend S& operator>>(S& in, Sequence& seq) {
    assert(seq.empty());
    auto sg1 = std::experimental::scope_fail([&]() { seq.clear(); });

    // read version
    uint8_t version = in.get();

    // return successfully, if stream is empty
    if (in.eof())
      return in;

    // check version
    if (version != 1) {
      throw BareBones::Exception(0x0baacae87c11b49a, "Invalid Sequence version %d while reading from stream, only version 1 is supported", version);
    }

    auto original_exceptions = in.exceptions();
    auto sg2 = std::experimental::scope_exit([&]() { in.exceptions(original_exceptions); });
    in.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit);

    // read size
    in.read(reinterpret_cast<char*>(&seq.size_), sizeof(seq.size_));

    // read is completed, if there are no items
    if (!seq.size_) {
      return in;
    }

    auto new_size = seq.next_size(seq.size_);

    // read keys
    seq.reserve_keys(new_size);
    in.read(reinterpret_cast<char*>(static_cast<uint8_t*>(seq.keys_)), keys_size(seq.size_));

    // calculate data size
    auto actual_data_size = Codec::decode_data_size(seq.size_, static_cast<const uint8_t*>(seq.keys_));
    if (actual_data_size > std::numeric_limits<uint32_t>::max()) {
      throw BareBones::Exception(0xe2421936fe194169, "The calculated size (%d) of Sequence exceeds uint32_t max value", actual_data_size);
    }

    // read data
    seq.reserve_data(new_size, actual_data_size);
    in.read(reinterpret_cast<char*>(static_cast<uint8_t*>(seq.data_)), actual_data_size);

    return in;
  }
};

template <class Codec, class KIteratorType, class DIteratorType>
inline __attribute__((always_inline)) auto encoder(KIteratorType k_i, DIteratorType d_i) noexcept {
  return EncodeIterator<Codec, KIteratorType, DIteratorType>(k_i, d_i);
}

template <class Codec, class ContainerType>
  requires std::is_same_v<ContainerType, std::vector<uint8_t>> || std::is_same_v<ContainerType, Vector<uint8_t>>
inline __attribute__((always_inline)) auto back_inserter(ContainerType& c, uint32_t size) noexcept {
  /**
   * ATTENTION! This function expects that c.push_back (indirectly called from std::back_inserter) when called
   * after reserve doesn't invalidate iterators!
   */

  auto original_size = c.size();

  c.reserve(original_size + keys_size(size) + size * sizeof(typename Codec::value_type));
  c.resize(original_size + keys_size(size));

  return encoder<Codec>(c.begin() + original_size, std::back_inserter(c));
}

template <class Codec, std::random_access_iterator InnerIteratorType>
  requires std::is_same<typename std::iterator_traits<InnerIteratorType>::value_type, uint8_t>::value
inline __attribute__((always_inline)) auto decoder(InnerIteratorType i, uint32_t size) noexcept {
  return std::pair(DecodeIterator<Codec, InnerIteratorType>(i, size), DecodeIteratorSentinel());
}

}  // namespace StreamVByte

template <class C>
struct IsTriviallyReallocatable<StreamVByte::Sequence<C>> : std::true_type {};

template <class C>
struct IsZeroInitializable<StreamVByte::Sequence<C>> : std::true_type {};

}  // namespace BareBones
