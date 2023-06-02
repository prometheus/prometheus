#pragma once

#include <bit>
#include <utility>

#include "bit.h"
#include "bit_sequence.h"
#include "encoding.h"
#include "type_traits.h"
#include "zigzag.h"

namespace BareBones {
namespace Encoding::Gorilla {

constexpr double STALE_NAN = std::bit_cast<double>(0x7ff0000000000002ull);

inline __attribute__((always_inline)) bool isstalenan(double v) noexcept {
  return std::bit_cast<uint64_t>(v) == 0x7ff0000000000002ull;
}

class __attribute__((__packed__)) StreamEncoder {
  int64_t last_ts_;
  int64_t last_ts_delta_;  // for stream gorilla samples might not be ordered

  double last_v_;
  uint8_t last_v_xor_length_ = 0;
  uint8_t last_v_xor_leading_z_;
  uint8_t last_v_xor_trailing_z_;
  uint8_t v_xor_waste_bits_written_;

  enum : uint8_t { INITIAL = 0, FIRST_ENCODED = 1, SECOND_ENCODED = 2 } state_ = INITIAL;

  friend class StreamDecoder;

  inline __attribute__((always_inline)) void encode_value(double v, BitSequence& bitseq) noexcept {
    uint64_t v_xor = std::bit_cast<uint64_t>(last_v_) ^ std::bit_cast<uint64_t>(v);

    last_v_ = v;

    if (v_xor == 0) {
      bitseq.push_back_single_zero_bit();
      return;
    }

    uint8_t v_xor_leading_z = std::countl_zero(v_xor);
    uint8_t v_xor_trailing_z = std::countr_zero(v_xor);

    // we store lead_z in 5bits in encoding, so it's limited by 31
    v_xor_leading_z = v_xor_leading_z > 31 ? 31 : v_xor_leading_z;

    uint8_t v_xor_length = 64 - v_xor_leading_z - v_xor_trailing_z;

    // we need to write xor length, if it was never written
    if (last_v_xor_length_ == 0)
      goto write_xor_length;

    // we need to write xor length, if xor doesn't fit into the same bit range
    if (v_xor_leading_z < last_v_xor_leading_z_ || v_xor_trailing_z < last_v_xor_trailing_z_)
      goto write_xor_length;

    // heuristics that optimizes gorilla size based on one-time length change or amount of unnecessary bits written
    {
      // always positive, because we already checked that xor fits into the same bit range
      uint8_t v_xor_length_delta = last_v_xor_length_ - v_xor_length;

      // we need to write xor length
      //  * either because of accumulated statistics (more than 50 waste bits were written since last xor length write)
      //  * or because of one time drastic change (length is smaller for more than 11 bits)
      if (v_xor_waste_bits_written_ >= 50 || v_xor_length_delta >= 11)
        goto write_xor_length;

      // we zero waste bits if length difference is less than 3
      v_xor_waste_bits_written_ = v_xor_length_delta < 3 ? 0 : v_xor_waste_bits_written_;

      // count unnecessary bits
      v_xor_waste_bits_written_ += v_xor_length_delta;
    }

    // if we got here we don't need to write xor length
    bitseq.push_back_bits_u32(2, 0b01);

    bitseq.push_back_bits_u64(last_v_xor_length_, v_xor >> last_v_xor_trailing_z_);
    return;

  write_xor_length:
    v_xor_waste_bits_written_ = 0;
    last_v_xor_length_ = v_xor_length;
    last_v_xor_leading_z_ = v_xor_leading_z;
    last_v_xor_trailing_z_ = v_xor_trailing_z;
    assert(last_v_xor_length_ + last_v_xor_trailing_z_ <= 64);

    bitseq.push_back_bits_u32(1 + 1 + 5 + 6, 0b11 | (v_xor_leading_z << (1 + 1)) | (v_xor_length << (1 + 1 + 5)));
    bitseq.push_back_bits_u64(last_v_xor_length_, v_xor >> last_v_xor_trailing_z_);
  }

 public:
  inline __attribute__((always_inline)) void encode(int64_t ts, double v, BitSequence& ts_bitseq, BitSequence& v_bitseq) noexcept {
    if (state_ == INITIAL) {
      last_ts_ = ts;

      ts_bitseq.push_back_u64_svbyte_0248(ZigZag::encode(last_ts_));

      last_v_ = v;

      v_bitseq.push_back_d64_svbyte_0468(v);

      state_ = FIRST_ENCODED;
    } else if (state_ == FIRST_ENCODED) {
      last_ts_delta_ = ts - last_ts_;
      last_ts_ = ts;

      ts_bitseq.push_back_u64_svbyte_2468(ZigZag::encode(last_ts_delta_));

      encode_value(v, v_bitseq);

      state_ = SECOND_ENCODED;
    } else {
      const int64_t ts_delta = ts - last_ts_;
      const uint64_t ts_dod_zigzag = ZigZag::encode(ts_delta - last_ts_delta_);

      last_ts_delta_ = ts_delta;
      last_ts_ = ts;

      if (ts_dod_zigzag == 0) {
        ts_bitseq.push_back_single_zero_bit();
      } else {
        uint8_t ts_dod_significant_len = 64 - std::countl_zero(ts_dod_zigzag);

        if (ts_dod_significant_len <= 5) {
          // 1->0
          ts_bitseq.push_back_bits_u32(2 + 5, 0b01 | (ts_dod_zigzag << 2));
        } else if (ts_dod_significant_len <= 15) {
          // 1->1->0
          ts_bitseq.push_back_bits_u32(3 + 15, 0b011 | (ts_dod_zigzag << 3));
        } else if (ts_dod_significant_len <= 18) {
          // 1->1->1->0
          ts_bitseq.push_back_bits_u32(4 + 18, 0b0111 | (ts_dod_zigzag << 4));
        } else {
          // 1->1->1->1
          ts_bitseq.push_back_bits_u32(4, 0b1111);
          ts_bitseq.push_back_u64_svbyte_2468(ts_dod_zigzag);
        }
      }

      encode_value(v, v_bitseq);
    }
  }
};

static_assert(sizeof(StreamEncoder) == 29);

class __attribute__((__packed__)) StreamDecoder {
  int64_t last_ts_;
  int64_t last_ts_delta_;  // for stream gorilla samples might not be ordered

  double last_v_;
  uint8_t last_v_xor_length_;
  uint8_t last_v_xor_trailing_z_;

  enum : uint8_t { INITIAL = 0, FIRST_DECODED = 1, SECOND_DECODED = 2 } state_ = INITIAL;

  inline __attribute__((always_inline)) void decode_value(BitSequence::Reader& bitseq) noexcept {
    const uint32_t buf = bitseq.read_u32();

    // value not changed?
    if ((buf & 0b1) == 0) {
      bitseq.ff(1);
      return;
    }

    bitseq.ff(1 + 1);

    // length changed?
    if (buf & 0b10) {
      bitseq.ff(5 + 6);

      uint8_t v_xor_leading_z = Bit::bextr(buf, 2, 5);
      last_v_xor_length_ = Bit::bextr(buf, 7, 6);

      // if last_v_xor_length is zero it should be 64
      last_v_xor_length_ += !last_v_xor_length_ * 64;

      last_v_xor_trailing_z_ = 64 - v_xor_leading_z - last_v_xor_length_;
    }

    const uint64_t v_xor = bitseq.consume_bits_u64(last_v_xor_length_) << last_v_xor_trailing_z_;
    const uint64_t v = std::bit_cast<uint64_t>(last_v_) ^ v_xor;
    last_v_ = std::bit_cast<double>(v);
  }

 public:
  StreamDecoder() = default;

  // TODO: merge move ctor and copy ctor into one?
  inline __attribute__((always_inline)) StreamDecoder(const StreamEncoder& e)
      : last_ts_(e.last_ts_),
        last_ts_delta_(e.last_ts_delta_),
        last_v_(e.last_v_),
        last_v_xor_length_(e.last_v_xor_length_),
        last_v_xor_trailing_z_(e.last_v_xor_trailing_z_),
        state_(static_cast<decltype(this->state_)>(e.state_)) {}

  inline __attribute__((always_inline)) StreamDecoder(StreamEncoder&& e)
      : last_ts_(e.last_ts_),
        last_ts_delta_(e.last_ts_delta_),
        last_v_(e.last_v_),
        last_v_xor_length_(std::move(e.last_v_xor_length_)),
        last_v_xor_trailing_z_(std::move(e.last_v_xor_trailing_z_)),
        state_(static_cast<decltype(this->state_)>(e.state_)) {}

  inline __attribute__((always_inline)) int64_t last_timestamp() const noexcept { return last_ts_; }

  inline __attribute__((always_inline)) double last_value() const noexcept { return last_v_; }

  inline __attribute__((always_inline)) void decode(int64_t ts, BitSequence::Reader& v_bitseq) noexcept {
    if (__builtin_expect(state_ == INITIAL, false)) {
      last_ts_ = ts;

      last_v_ = v_bitseq.consume_d64_svbyte_0468();

      state_ = FIRST_DECODED;
    } else if (__builtin_expect(state_ == FIRST_DECODED, false)) {
      last_ts_delta_ = std::bit_cast<int64_t>(ts) - std::bit_cast<int64_t>(last_ts_);
      last_ts_ = ts;

      decode_value(v_bitseq);

      state_ = SECOND_DECODED;
    } else {
      last_ts_delta_ = std::bit_cast<int64_t>(ts) - std::bit_cast<int64_t>(last_ts_);
      last_ts_ = ts;

      decode_value(v_bitseq);
    }
  }

  inline __attribute__((always_inline)) void decode(BitSequence::Reader& ts_bitseq, BitSequence::Reader& v_bitseq) noexcept {
    if (__builtin_expect(state_ == INITIAL, false)) {
      last_ts_ = ZigZag::decode(ts_bitseq.consume_u64_svbyte_0248());

      last_v_ = v_bitseq.consume_d64_svbyte_0468();

      state_ = FIRST_DECODED;
    } else if (__builtin_expect(state_ == FIRST_DECODED, false)) {
      last_ts_delta_ = ZigZag::decode(ts_bitseq.consume_u64_svbyte_2468());
      last_ts_ += last_ts_delta_;

      decode_value(v_bitseq);

      state_ = SECOND_DECODED;
    } else {
      uint32_t buf = ts_bitseq.read_u32();

      if (buf & 0b1) {
        uint64_t dod_zigzag;

        if ((buf & 0b10) == 0) {
          // 1->0 -> 5bit
          dod_zigzag = Bit::bextr(buf, 2, 5);
          ts_bitseq.ff(2 + 5);
        } else if ((buf & 0b100) == 0) {
          // 1->1->0 -> 15bit
          dod_zigzag = Bit::bextr(buf, 3, 15);
          ts_bitseq.ff(3 + 15);
        } else if ((buf & 0b1000) == 0) {
          // 1->1->1->0 -> 18bit
          dod_zigzag = Bit::bextr(buf, 4, 18);
          ts_bitseq.ff(4 + 18);
        } else {
          // 1->1->1->1 -> 64bit
          ts_bitseq.ff(4);
          dod_zigzag = ts_bitseq.consume_u64_svbyte_2468();
        }

        last_ts_delta_ += ZigZag::decode(dod_zigzag);
      } else {
        ts_bitseq.ff(1);
      }

      last_ts_ += last_ts_delta_;

      decode_value(v_bitseq);
    }
  }

  template <class OutputStream>
  void save(OutputStream& out) {
    out.write(reinterpret_cast<const char*>(&last_ts_), sizeof(last_ts_));
    out.write(reinterpret_cast<const char*>(&last_ts_delta_), sizeof(last_ts_delta_));
    out.write(reinterpret_cast<const char*>(&last_v_), sizeof(last_v_));
    out.write(reinterpret_cast<const char*>(&last_v_xor_length_), sizeof(last_v_xor_length_));
    out.write(reinterpret_cast<const char*>(&last_v_xor_trailing_z_), sizeof(last_v_xor_trailing_z_));
    out.write(reinterpret_cast<const char*>(&state_), sizeof(state_));
  }

  template <class InputStream>
  void load(InputStream& in) {
    in.read(reinterpret_cast<char*>(&last_ts_), sizeof(last_ts_));
    in.read(reinterpret_cast<char*>(&last_ts_delta_), sizeof(last_ts_delta_));
    in.read(reinterpret_cast<char*>(&last_v_), sizeof(last_v_));
    in.read(reinterpret_cast<char*>(&last_v_xor_length_), sizeof(last_v_xor_length_));
    in.read(reinterpret_cast<char*>(&last_v_xor_trailing_z_), sizeof(last_v_xor_trailing_z_));
    in.read(reinterpret_cast<char*>(&state_), sizeof(state_));
  }

  bool operator==(const StreamDecoder& decoder) const = default;
};

static_assert(sizeof(StreamDecoder) == 27);

}  // namespace Encoding::Gorilla

template <>
struct IsTriviallyReallocatable<Encoding::Gorilla::StreamEncoder> : std::true_type {};

template <>
struct IsZeroInitializable<Encoding::Gorilla::StreamEncoder> : std::true_type {};

template <>
struct IsTriviallyDestructible<Encoding::Gorilla::StreamEncoder> : std::true_type {};

template <>
struct IsTriviallyReallocatable<Encoding::Gorilla::StreamDecoder> : std::true_type {};

template <>
struct IsZeroInitializable<Encoding::Gorilla::StreamDecoder> : std::true_type {};

template <>
struct IsTriviallyDestructible<Encoding::Gorilla::StreamDecoder> : std::true_type {};

}  // namespace BareBones
