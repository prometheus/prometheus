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

template <class TimestampEncoder>
concept TimestampEncoderInterface = requires(BitSequence& sequence) {
  { TimestampEncoder::encode(int64_t(), sequence) };
  { TimestampEncoder::encode_delta(int64_t(), sequence) };
  { TimestampEncoder::encode_delta_of_delta(int64_t(), sequence) };
};

class ZigZagTimestampEncoder {
 public:
  PROMPP_ALWAYS_INLINE static void encode(int64_t ts, BitSequence& stream) { stream.push_back_u64_svbyte_0248(ZigZag::encode(ts)); }
  PROMPP_ALWAYS_INLINE static void encode_delta(int64_t delta, BitSequence& stream) { stream.push_back_u64_svbyte_2468(ZigZag::encode(delta)); }
  PROMPP_ALWAYS_INLINE static void encode_delta_of_delta(int64_t delta_of_delta, BitSequence& stream) {
    const uint64_t ts_dod_zigzag = ZigZag::encode(delta_of_delta);

    if (ts_dod_zigzag == 0) {
      stream.push_back_single_zero_bit();
    } else {
      uint8_t ts_dod_significant_len = 64 - std::countl_zero(ts_dod_zigzag);

      if (ts_dod_significant_len <= 5) {
        // 1->0
        stream.push_back_bits_u32(2 + 5, 0b01 | (ts_dod_zigzag << 2));
      } else if (ts_dod_significant_len <= 15) {
        // 1->1->0
        stream.push_back_bits_u32(3 + 15, 0b011 | (ts_dod_zigzag << 3));
      } else if (ts_dod_significant_len <= 18) {
        // 1->1->1->0
        stream.push_back_bits_u32(4 + 18, 0b0111 | (ts_dod_zigzag << 4));
      } else {
        // 1->1->1->1
        stream.push_back_bits_u32(4, 0b1111);
        stream.push_back_u64_svbyte_2468(ts_dod_zigzag);
      }
    }
  }
};

static constexpr size_t kMaxVarintLength = 10;

class TimestampEncoder {
 public:
  PROMPP_ALWAYS_INLINE static void encode(int64_t ts, BitSequence& stream) {
    uint8_t varint_buffer[kMaxVarintLength]{};
    push_varint_buffer(varint_buffer, write_varint(varint_buffer, ts), stream);
  }
  PROMPP_ALWAYS_INLINE static void encode_delta(int64_t delta, BitSequence& stream) {
    uint8_t varint_buffer[kMaxVarintLength]{};
    push_varint_buffer(varint_buffer, write_varint(varint_buffer, std::bit_cast<uint64_t>(delta)), stream);
  }
  PROMPP_ALWAYS_INLINE static void encode_delta_of_delta(int64_t delta_of_delta, BitSequence& stream) {
    const auto ts_dod_zigzag = std::bit_cast<uint64_t>(delta_of_delta);

    if (ts_dod_zigzag == 0) {
      stream.push_back_single_zero_bit();
    } else {
      uint8_t ts_dod_significant_len = 64 - std::countl_zero(ts_dod_zigzag);

      if (ts_dod_significant_len <= 4) {
        // 1->0
        stream.push_back_bits_u32(2 + 4, 0b01 | (ts_dod_zigzag << 2));
      } else if (ts_dod_significant_len <= 14) {
        // 1->1->0
        stream.push_back_bits_u32(3 + 14, 0b011 | (ts_dod_zigzag << 3));
      } else if (ts_dod_significant_len <= 17) {
        // 1->1->1->0
        stream.push_back_bits_u32(4 + 17, 0b0111 | (ts_dod_zigzag << 4));
      } else {
        // 1->1->1->1
        stream.push_back_bits_u32(4, 0b1111);
        stream.push_back_u64(ts_dod_zigzag);
      }
    }
  }

 private:
  PROMPP_ALWAYS_INLINE static void push_varint_buffer(const uint8_t* buffer, size_t bytes, BitSequence& stream) {
    if (bytes <= sizeof(uint64_t)) {
      stream.push_back_bits_u64(bytes * 8, *reinterpret_cast<const uint64_t*>(buffer));
    } else {
      stream.push_back_bits_u64(sizeof(uint64_t) * 8, *reinterpret_cast<const uint64_t*>(buffer));
      stream.push_back_bits_u32((bytes - sizeof(uint64_t)) * 8, *reinterpret_cast<const uint16_t*>(buffer + sizeof(uint64_t)));
    }
  }

  PROMPP_ALWAYS_INLINE static size_t write_varint(uint8_t* data, int64_t value) {
    auto uint_value = std::bit_cast<uint64_t>(value) << 1;
    if (value < 0) {
      uint_value = ~uint_value;
    }

    return write_varint(data, uint_value);
  }

  PROMPP_ALWAYS_INLINE static size_t write_varint(uint8_t* data, uint64_t value) {
    auto p = data;
    while (value >= 128) {
      *p++ = 0x80 | (value & 0x7f);
      value >>= 7;
    }
    *p++ = static_cast<uint8_t>(value);
    return p - data;
  }
};

template <class TimestampDecoder>
concept TimestampDecoderInterface = requires(BitSequence::Reader& reader) {
  { TimestampDecoder::decode(reader) } -> std::same_as<int64_t>;
  { TimestampDecoder::decode_delta(reader) } -> std::same_as<int64_t>;
  { TimestampDecoder::decode_delta_of_delta(uint32_t(), reader) } -> std::same_as<int64_t>;
};

class ZigZagTimestampDecoder {
 public:
  PROMPP_ALWAYS_INLINE static int64_t decode(BitSequence::Reader& reader) { return ZigZag::decode(reader.consume_u64_svbyte_0248()); }
  PROMPP_ALWAYS_INLINE static int64_t decode_delta(BitSequence::Reader& reader) { return ZigZag::decode(reader.consume_u64_svbyte_2468()); }
  PROMPP_ALWAYS_INLINE static int64_t decode_delta_of_delta(uint32_t buf, BitSequence::Reader& reader) {
    uint64_t dod_zigzag;

    if ((buf & 0b10) == 0) {
      // 1->0 -> 5bit
      dod_zigzag = Bit::bextr(buf, 2, 5);
      reader.ff(2 + 5);
    } else if ((buf & 0b100) == 0) {
      // 1->1->0 -> 15bit
      dod_zigzag = Bit::bextr(buf, 3, 15);
      reader.ff(3 + 15);
    } else if ((buf & 0b1000) == 0) {
      // 1->1->1->0 -> 18bit
      dod_zigzag = Bit::bextr(buf, 4, 18);
      reader.ff(4 + 18);
    } else {
      // 1->1->1->1 -> 64bit
      reader.ff(4);
      dod_zigzag = reader.consume_u64_svbyte_2468();
    }

    return ZigZag::decode(dod_zigzag);
  }
};

class TimestampDecoder {
 public:
  PROMPP_ALWAYS_INLINE static int64_t decode(BitSequence::Reader& reader) { return read_varint(reader); }
  PROMPP_ALWAYS_INLINE static int64_t decode_delta(BitSequence::Reader& reader) { return std::bit_cast<int64_t>(read_var_uint(reader)); }
  PROMPP_ALWAYS_INLINE static int64_t decode_delta_of_delta(uint32_t buf, BitSequence::Reader& reader) {
    uint64_t dod_zigzag;

    if ((buf & 0b10) == 0) {
      // 1->0 -> 4bit
      dod_zigzag = Bit::bextr(buf, 2, 4);
      reader.ff(2 + 4);
    } else if ((buf & 0b100) == 0) {
      // 1->1->0 -> 14bit
      dod_zigzag = Bit::bextr(buf, 3, 14);
      reader.ff(3 + 14);
    } else if ((buf & 0b1000) == 0) {
      // 1->1->1->0 -> 17bit
      dod_zigzag = Bit::bextr(buf, 4, 17);
      reader.ff(4 + 17);
    } else {
      // 1->1->1->1 -> 64bit
      reader.ff(4);
      dod_zigzag = reader.consume_u64();
    }

    return std::bit_cast<int64_t>(dod_zigzag);
  }

 private:
  PROMPP_ALWAYS_INLINE static int64_t read_varint(BitSequence::Reader& reader) {
    auto value = read_var_uint(reader);
    auto result = std::bit_cast<int64_t>(value >> 1);
    if ((value & 1) != 0) {
      result = ~result;
    }

    return result;
  }

  PROMPP_ALWAYS_INLINE static uint64_t read_var_uint(BitSequence::Reader& reader) {
    uint64_t result = 0;
    uint8_t shift = 0;

    for (size_t i = 0; i < kMaxVarintLength; ++i) {
      auto byte = static_cast<uint64_t>(reader.consume_bits_u32(8));
      if (byte < 0x80) {
        return result | static_cast<uint64_t>(byte) << shift;
      }

      result |= (byte & 0x7F) << shift;
      shift += 7;
    }

    return result;
  }
};

template <TimestampDecoderInterface T>
class StreamDecoder;

template <TimestampEncoderInterface TimestampEncoder>
class __attribute__((__packed__)) StreamEncoder {
  int64_t last_ts_;
  int64_t last_ts_delta_;  // for stream gorilla samples might not be ordered

  double last_v_;
  uint8_t last_v_xor_length_{};
  uint8_t last_v_xor_leading_z_{};
  uint8_t last_v_xor_trailing_z_{};
  uint8_t v_xor_waste_bits_written_{};

  enum : uint8_t { INITIAL = 0, FIRST_ENCODED = 1, SECOND_ENCODED = 2 } state_ = INITIAL;

  template <TimestampDecoderInterface T>
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

      TimestampEncoder::encode(last_ts_, ts_bitseq);

      last_v_ = v;

      v_bitseq.push_back_d64_svbyte_0468(v);

      state_ = FIRST_ENCODED;
    } else if (state_ == FIRST_ENCODED) {
      last_ts_delta_ = ts - last_ts_;
      last_ts_ = ts;

      TimestampEncoder::encode_delta(last_ts_delta_, ts_bitseq);

      encode_value(v, v_bitseq);

      state_ = SECOND_ENCODED;
    } else {
      const int64_t ts_delta = ts - last_ts_;

      TimestampEncoder::encode_delta_of_delta(ts_delta - last_ts_delta_, ts_bitseq);

      last_ts_delta_ = ts_delta;
      last_ts_ = ts;

      encode_value(v, v_bitseq);
    }
  }
};

static_assert(sizeof(StreamEncoder<ZigZagTimestampEncoder>) == 29);

template <TimestampDecoderInterface TimestampDecoder>
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
  template <class StreamEncoder>
  explicit inline __attribute__((always_inline)) StreamDecoder(const StreamEncoder& e)
      : last_ts_(e.last_ts_),
        last_ts_delta_(e.last_ts_delta_),
        last_v_(e.last_v_),
        last_v_xor_length_(e.last_v_xor_length_),
        last_v_xor_trailing_z_(e.last_v_xor_trailing_z_),
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
      last_ts_ = TimestampDecoder::decode(ts_bitseq);

      last_v_ = v_bitseq.consume_d64_svbyte_0468();

      state_ = FIRST_DECODED;
    } else if (__builtin_expect(state_ == FIRST_DECODED, false)) {
      last_ts_delta_ = TimestampDecoder::decode_delta(ts_bitseq);
      last_ts_ += last_ts_delta_;

      decode_value(v_bitseq);

      state_ = SECOND_DECODED;
    } else {
      uint32_t buf = ts_bitseq.read_u32();

      if (buf & 0b1) {
        last_ts_delta_ += TimestampDecoder::decode_delta_of_delta(buf, ts_bitseq);
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

static_assert(sizeof(StreamDecoder<ZigZagTimestampDecoder>) == 27);

}  // namespace Encoding::Gorilla

template <Encoding::Gorilla::TimestampEncoderInterface TimestampEncoder>
struct IsTriviallyReallocatable<Encoding::Gorilla::StreamEncoder<TimestampEncoder>> : std::true_type {};

template <Encoding::Gorilla::TimestampEncoderInterface TimestampEncoder>
struct IsZeroInitializable<Encoding::Gorilla::StreamEncoder<TimestampEncoder>> : std::true_type {};

template <Encoding::Gorilla::TimestampEncoderInterface TimestampEncoder>
struct IsTriviallyDestructible<Encoding::Gorilla::StreamEncoder<TimestampEncoder>> : std::true_type {};

template <Encoding::Gorilla::TimestampDecoderInterface TimestampDecoder>
struct IsTriviallyReallocatable<Encoding::Gorilla::StreamDecoder<TimestampDecoder>> : std::true_type {};

template <Encoding::Gorilla::TimestampDecoderInterface TimestampDecoder>
struct IsZeroInitializable<Encoding::Gorilla::StreamDecoder<TimestampDecoder>> : std::true_type {};

template <Encoding::Gorilla::TimestampDecoderInterface TimestampDecoder>
struct IsTriviallyDestructible<Encoding::Gorilla::StreamDecoder<TimestampDecoder>> : std::true_type {};

}  // namespace BareBones
