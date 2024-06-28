#pragma once

#include <bit>
#include <utility>

#include "bit.h"
#include "encoding.h"
#include "type_traits.h"
#include "varint.h"
#include "zigzag.h"

namespace BareBones {
namespace Encoding::Gorilla {

enum class ValueType : uint8_t {
  kStaleNan = 0,
  kValue,
};

constexpr double STALE_NAN = std::bit_cast<double>(0x7ff0000000000002ull);

constexpr PROMPP_ALWAYS_INLINE bool isstalenan(double v) noexcept {
  return std::bit_cast<uint64_t>(v) == 0x7ff0000000000002ull;
}

constexpr PROMPP_ALWAYS_INLINE ValueType get_value_type(double value) noexcept {
  return isstalenan(value) ? ValueType::kStaleNan : ValueType::kValue;
}

struct PROMPP_ATTRIBUTE_PACKED TimestampEncoderState {
  int64_t last_ts{};
  int64_t last_ts_delta{};  // for stream gorilla samples might not be ordered

  bool operator==(const TimestampEncoderState&) const noexcept = default;

  PROMPP_ALWAYS_INLINE void reset() noexcept {
    last_ts = {};
    last_ts_delta = {};
  }
};

template <class TimestampEncoder>
concept TimestampEncoderInterface = requires(TimestampEncoderState& state, BitSequence& sequence) {
  { TimestampEncoder::encode(state, int64_t(), sequence) } -> std::same_as<void>;
  { TimestampEncoder::encode_delta(state, int64_t(), sequence) } -> std::same_as<void>;
  { TimestampEncoder::encode_delta_of_delta(state, int64_t(), sequence) } -> std::same_as<void>;
};

struct DodSignificantLengths {
  uint8_t first;
  uint8_t second;
  uint8_t third;
};

static constexpr DodSignificantLengths kDefaultDogSignificantLengths = DodSignificantLengths{.first = 5, .second = 15, .third = 18};

template <DodSignificantLengths kDogSignificantLengths = kDefaultDogSignificantLengths>
class ZigZagTimestampEncoder {
 public:
  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode(TimestampEncoderState& state, int64_t ts, BitSequence& stream) {
    state.last_ts = ts;

    stream.push_back_u64_svbyte_0248(ZigZag::encode(ts));
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode_delta(TimestampEncoderState& state, int64_t ts, BitSequence& stream) {
    state.last_ts_delta = ts - state.last_ts;
    state.last_ts = ts;

    stream.push_back_u64_svbyte_2468(ZigZag::encode(state.last_ts_delta));
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode_delta_of_delta(TimestampEncoderState& state, int64_t ts, BitSequence& stream) {
    auto ts_delta = ts - state.last_ts;
    const int64_t delta_of_delta = ts_delta - state.last_ts_delta;
    const uint64_t ts_dod_zigzag = ZigZag::encode(delta_of_delta);

    encode_delta_of_delta<BitSequence>(ts_dod_zigzag, stream);

    state.last_ts_delta = ts_delta;
    state.last_ts = ts;
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode_delta_of_delta(uint64_t ts_dod_zigzag, BitSequence& stream) {
    if (ts_dod_zigzag == 0) {
      stream.push_back_single_zero_bit();
    } else {
      uint8_t ts_dod_significant_len = 64 - std::countl_zero(ts_dod_zigzag);

      if (ts_dod_significant_len <= kDogSignificantLengths.first) {
        // 1->0
        stream.push_back_bits_u32(2 + kDogSignificantLengths.first, 0b01 | (ts_dod_zigzag << 2));
      } else if (ts_dod_significant_len <= kDogSignificantLengths.second) {
        // 1->1->0
        stream.push_back_bits_u32(3 + kDogSignificantLengths.second, 0b011 | (ts_dod_zigzag << 3));
      } else if (ts_dod_significant_len <= kDogSignificantLengths.third) {
        // 1->1->1->0
        stream.push_back_bits_u32(4 + kDogSignificantLengths.third, 0b0111 | (ts_dod_zigzag << 4));
      } else {
        // 1->1->1->1
        stream.push_back_bits_u32(4, 0b1111);
        stream.push_back_u64_svbyte_2468(ts_dod_zigzag);
      }
    }
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode_delta_of_delta_with_stale_nan(TimestampEncoderState& state, double timestamp, BitSequence& stream) {
    if (isstalenan(timestamp)) {
      [[unlikely]];
      stream.push_back_bits_u32(5, 0b11111);
    } else {
      auto ts = static_cast<int64_t>(timestamp);
      auto ts_delta = ts - state.last_ts;
      const int64_t delta_of_delta = ts_delta - state.last_ts_delta;
      const uint64_t ts_dod_zigzag = ZigZag::encode(delta_of_delta);

      encode_delta_of_delta_with_stale_nan<BitSequence>(ts_dod_zigzag, stream);

      state.last_ts_delta = ts_delta;
      state.last_ts = ts;
    }
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode_delta_of_delta_with_stale_nan(uint64_t ts_dod_zigzag, BitSequence& stream) {
    if (ts_dod_zigzag == 0) {
      stream.push_back_single_zero_bit();
    } else {
      uint8_t ts_dod_significant_len = 64 - std::countl_zero(ts_dod_zigzag);

      if (ts_dod_significant_len <= kDogSignificantLengths.first) {
        // 1->0
        stream.push_back_bits_u32(2 + kDogSignificantLengths.first, 0b01 | (ts_dod_zigzag << 2));
      } else if (ts_dod_significant_len <= kDogSignificantLengths.second) {
        // 1->1->0
        stream.push_back_bits_u32(3 + kDogSignificantLengths.second, 0b011 | (ts_dod_zigzag << 3));
      } else if (ts_dod_significant_len <= kDogSignificantLengths.third) {
        // 1->1->1->0
        stream.push_back_bits_u32(4 + kDogSignificantLengths.third, 0b0111 | (ts_dod_zigzag << 4));
      } else {
        // 1->1->1->1
        stream.push_back_bits_u32(5, 0b01111);
        stream.push_back_u64_svbyte_2468(ts_dod_zigzag);
      }
    }
  }
};

struct PROMPP_ATTRIBUTE_PACKED ValuesEncoderState {
  double last_v{};
  uint8_t last_v_xor_length{};
  uint8_t last_v_xor_leading_z{};
  uint8_t last_v_xor_trailing_z{};
  uint8_t v_xor_waste_bits_written{};

  bool operator==(const ValuesEncoderState&) const noexcept = default;

  PROMPP_ALWAYS_INLINE void reset() {
    last_v = {};
    last_v_xor_length = {};
    last_v_xor_leading_z = {};
    last_v_xor_trailing_z = {};
    v_xor_waste_bits_written = {};
  }
};

struct PROMPP_ATTRIBUTE_PACKED ValuesDecoderState {
  double last_v{};
  uint8_t last_v_xor_length{};
  uint8_t last_v_xor_trailing_z{};

  bool operator==(const ValuesDecoderState&) const noexcept = default;
};

enum class GorillaState : uint8_t {
  kFirstPoint = 0,
  kSecondPoint = 1,
  kOtherPoint = 2,
};

struct PROMPP_ATTRIBUTE_PACKED EncoderState {
  TimestampEncoderState timestamp_encoder;
  ValuesEncoderState values_encoder;
  GorillaState state{GorillaState::kFirstPoint};

  bool operator==(const EncoderState&) const noexcept = default;
};

struct PROMPP_ATTRIBUTE_PACKED DecoderState {
  TimestampEncoderState timestamp_decoder;
  ValuesDecoderState values_decoder;
  GorillaState state{GorillaState::kFirstPoint};

  DecoderState() = default;
  DecoderState(const DecoderState&) = default;
  DecoderState(DecoderState&&) noexcept = default;
  explicit DecoderState(const EncoderState& encoder_state)
      : timestamp_decoder(encoder_state.timestamp_encoder),
        values_decoder{.last_v = encoder_state.values_encoder.last_v,
                       .last_v_xor_length = encoder_state.values_encoder.last_v_xor_length,
                       .last_v_xor_trailing_z = encoder_state.values_encoder.last_v_xor_trailing_z},
        state(encoder_state.state) {}

  DecoderState& operator=(const DecoderState&) = default;
  DecoderState& operator=(DecoderState&&) noexcept = default;

  bool operator==(const DecoderState&) const noexcept = default;
};

class TimestampEncoder {
 public:
  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode(TimestampEncoderState& state, int64_t ts, BitSequence& stream) {
    state.last_ts = ts;

    uint8_t varint_buffer[VarInt::kMaxVarIntLength]{};
    push_varint_buffer(varint_buffer, VarInt::write(varint_buffer, ts), stream);
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode_delta(TimestampEncoderState& state, int64_t ts, BitSequence& stream) {
    state.last_ts_delta = ts - state.last_ts;
    state.last_ts = ts;

    uint8_t varint_buffer[VarInt::kMaxVarIntLength]{};
    push_varint_buffer(varint_buffer, VarInt::write(varint_buffer, std::bit_cast<uint64_t>(state.last_ts_delta)), stream);
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode_delta_of_delta(TimestampEncoderState& state, int64_t ts, BitSequence& stream) {
    auto ts_delta = ts - state.last_ts;
    const int64_t delta_of_delta = ts_delta - state.last_ts_delta;
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

    state.last_ts_delta = ts_delta;
    state.last_ts = ts;
  }

 private:
  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void push_varint_buffer(const uint8_t* buffer, size_t bytes, BitSequence& stream) {
    if (bytes <= sizeof(uint64_t)) {
      stream.push_back_bits_u64(BareBones::Bit::to_bits(bytes), *reinterpret_cast<const uint64_t*>(buffer));
    } else {
      stream.push_back_bits_u64(BareBones::Bit::to_bits(sizeof(uint64_t)), *reinterpret_cast<const uint64_t*>(buffer));
      stream.push_back_bits_u32(BareBones::Bit::to_bits(bytes - sizeof(uint64_t)), *reinterpret_cast<const uint16_t*>(buffer + sizeof(uint64_t)));
    }
  }
};

template <class TimestampDecoder>
concept TimestampDecoderInterface = requires(TimestampEncoderState& state, BitSequenceReader& reader) {
  { TimestampDecoder::decode(state, reader) } -> std::same_as<void>;
  { TimestampDecoder::decode_delta(state, reader) } -> std::same_as<void>;
  { TimestampDecoder::decode_delta_of_delta(state, reader) } -> std::same_as<void>;
};

template <DodSignificantLengths kDogSignificantLengths = kDefaultDogSignificantLengths>
class ZigZagTimestampDecoder {
 public:
  PROMPP_ALWAYS_INLINE static void decode(TimestampEncoderState& state, BitSequenceReader& reader) {
    state.last_ts = ZigZag::decode(reader.consume_u64_svbyte_0248());
  }
  PROMPP_ALWAYS_INLINE static void decode_delta(TimestampEncoderState& state, BitSequenceReader& reader) {
    state.last_ts_delta = ZigZag::decode(reader.consume_u64_svbyte_2468());
    state.last_ts += state.last_ts_delta;
  }
  PROMPP_ALWAYS_INLINE static void decode_delta_of_delta(TimestampEncoderState& state, BitSequenceReader& reader) {
    uint32_t buf = reader.read_u32();

    if (buf & 0b1) {
      uint64_t dod_zigzag;

      if ((buf & 0b10) == 0) {
        // 1->0 -> 5bit
        dod_zigzag = Bit::bextr(buf, 2, kDogSignificantLengths.first);
        reader.ff(2 + kDogSignificantLengths.first);
      } else if ((buf & 0b100) == 0) {
        // 1->1->0 -> 15bit
        dod_zigzag = Bit::bextr(buf, 3, kDogSignificantLengths.second);
        reader.ff(3 + kDogSignificantLengths.second);
      } else if ((buf & 0b1000) == 0) {
        // 1->1->1->0 -> 18bit
        dod_zigzag = Bit::bextr(buf, 4, kDogSignificantLengths.third);
        reader.ff(4 + kDogSignificantLengths.third);
      } else {
        // 1->1->1->1 -> 64bit
        reader.ff(4);
        dod_zigzag = reader.consume_u64_svbyte_2468();
      }

      state.last_ts_delta += ZigZag::decode(dod_zigzag);
    } else {
      reader.ff(1);
    }

    state.last_ts += state.last_ts_delta;
  }

  PROMPP_ALWAYS_INLINE static ValueType decode_delta_of_delta_with_stale_nan(TimestampEncoderState& state, BitSequenceReader& reader) {
    uint32_t buf = reader.read_u32();

    if (buf & 0b1) {
      uint64_t dod_zigzag;

      if ((buf & 0b10) == 0) {
        // 1->0 -> 5bit
        dod_zigzag = Bit::bextr(buf, 2, kDogSignificantLengths.first);
        reader.ff(2 + kDogSignificantLengths.first);
      } else if ((buf & 0b100) == 0) {
        // 1->1->0 -> 15bit
        dod_zigzag = Bit::bextr(buf, 3, kDogSignificantLengths.second);
        reader.ff(3 + kDogSignificantLengths.second);
      } else if ((buf & 0b1000) == 0) {
        // 1->1->1->0 -> 18bit
        dod_zigzag = Bit::bextr(buf, 4, kDogSignificantLengths.third);
        reader.ff(4 + kDogSignificantLengths.third);
      } else if ((buf & 0b10000) == 0) {
        // 1->1->1->1 -> 64bit
        reader.ff(5);
        dod_zigzag = reader.consume_u64_svbyte_2468();
      } else {
        reader.ff(5);
        return ValueType::kStaleNan;
      }

      state.last_ts_delta += ZigZag::decode(dod_zigzag);
    } else {
      reader.ff(1);
    }

    state.last_ts += state.last_ts_delta;
    return ValueType::kValue;
  }
};

class TimestampDecoder {
 public:
  PROMPP_ALWAYS_INLINE static void decode(TimestampEncoderState& state, BitSequenceReader& reader) { state.last_ts = VarInt::read(reader); }
  PROMPP_ALWAYS_INLINE static void decode_delta(TimestampEncoderState& state, BitSequenceReader& reader) {
    state.last_ts_delta = std::bit_cast<int64_t>(VarInt::read_uint(reader));
    state.last_ts += state.last_ts_delta;
  }
  static void decode_delta_of_delta(TimestampEncoderState& state, BitSequenceReader& reader) {
    uint32_t buf = reader.read_u32();

    if (buf & 0b1) {
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

      state.last_ts_delta += std::bit_cast<int64_t>(dod_zigzag);
    } else {
      reader.ff(1);
    }

    state.last_ts += state.last_ts_delta;
  }
};

template <class ValuesEncoder>
concept ValuesEncoderInterface = requires(ValuesEncoder& encoder, BitSequence& sequence, ValuesEncoderState& state) {
  { encoder.encode_first(state, double(), sequence) } -> std::same_as<void>;
  { encoder.encode(state, double(), sequence) } -> std::same_as<void>;
};

class ValuesEncoder {
 public:
  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode_first(ValuesEncoderState& state, double v, BitSequence& stream) noexcept {
    state.last_v = v;

    stream.push_back_d64_svbyte_0468(v);
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode_first(ValuesEncoderState& state, double v, uint32_t count, BitSequence& stream) noexcept {
    assert(count >= 1);

    encode_first(state, v, stream);
    stream.push_back_single_zero_bit(count - 1);
  }

  template <class BitSequence>
  static void encode(ValuesEncoderState& state, double v, BitSequence& stream) noexcept {
    uint64_t v_xor = std::bit_cast<uint64_t>(state.last_v) ^ std::bit_cast<uint64_t>(v);

    state.last_v = v;

    if (v_xor == 0) {
      stream.push_back_single_zero_bit();
      return;
    }

    uint8_t v_xor_leading_z = std::countl_zero(v_xor);
    uint8_t v_xor_trailing_z = std::countr_zero(v_xor);

    // we store lead_z in 5bits in encoding, so it's limited by 31
    v_xor_leading_z = v_xor_leading_z > 31 ? 31 : v_xor_leading_z;

    uint8_t v_xor_length = 64 - v_xor_leading_z - v_xor_trailing_z;

    // we need to write xor length, if it was never written
    if (state.last_v_xor_length == 0)
      goto write_xor_length;

    // we need to write xor length, if xor doesn't fit into the same bit range
    if (v_xor_leading_z < state.last_v_xor_leading_z || v_xor_trailing_z < state.last_v_xor_trailing_z)
      goto write_xor_length;

    // heuristics that optimizes gorilla size based on one-time length change or amount of unnecessary bits written
    {
      // always positive, because we already checked that xor fits into the same bit range
      uint8_t v_xor_length_delta = state.last_v_xor_length - v_xor_length;

      // we need to write xor length
      //  * either because of accumulated statistics (more than 50 waste bits were written since last xor length write)
      //  * or because of one time drastic change (length is smaller for more than 11 bits)
      if (state.v_xor_waste_bits_written >= 50 || v_xor_length_delta >= 11)
        goto write_xor_length;

      // we zero waste bits if length difference is less than 3
      state.v_xor_waste_bits_written = v_xor_length_delta < 3 ? 0 : state.v_xor_waste_bits_written;

      // count unnecessary bits
      state.v_xor_waste_bits_written += v_xor_length_delta;
    }

    // if we got here we don't need to write xor length
    stream.push_back_bits_u32(2, 0b01);

    stream.push_back_bits_u64(state.last_v_xor_length, v_xor >> state.last_v_xor_trailing_z);
    return;

  write_xor_length:
    state.v_xor_waste_bits_written = 0;
    state.last_v_xor_length = v_xor_length;
    state.last_v_xor_leading_z = v_xor_leading_z;
    state.last_v_xor_trailing_z = v_xor_trailing_z;
    assert(state.last_v_xor_length + state.last_v_xor_trailing_z <= 64);

    stream.push_back_bits_u32(1 + 1 + 5 + 6, 0b11 | (v_xor_leading_z << (1 + 1)) | (v_xor_length << (1 + 1 + 5)));
    stream.push_back_bits_u64(state.last_v_xor_length, v_xor >> state.last_v_xor_trailing_z);
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode(ValuesEncoderState& state, double v, uint32_t count, BitSequence& stream) noexcept {
    assert(count >= 1);

    encode(state, v, stream);
    stream.push_back_single_zero_bit(count - 1);
  }
};

template <class ValuesDecoder>
concept ValuesDecoderInterface = requires(ValuesDecoder& decoder, BitSequenceReader& reader, ValuesDecoderState& state) {
  { decoder.decode_first(state, reader) } -> std::same_as<void>;
  { decoder.decode(state, reader) } -> std::same_as<void>;
};

class ValuesDecoder {
 public:
  PROMPP_ALWAYS_INLINE static void decode_first(ValuesDecoderState& state, BitSequenceReader& reader) noexcept {
    state.last_v = reader.consume_d64_svbyte_0468();
  }

  static void decode(ValuesDecoderState& state, BitSequenceReader& reader) noexcept {
    const uint32_t buf = reader.read_u32();

    // value not changed?
    if ((buf & 0b1) == 0) {
      reader.ff(1);
      return;
    }

    reader.ff(1 + 1);

    // length changed?
    if (buf & 0b10) {
      reader.ff(5 + 6);

      uint8_t v_xor_leading_z = Bit::bextr(buf, 2, 5);
      state.last_v_xor_length = Bit::bextr(buf, 7, 6);

      // if last_v_xor_length is zero it should be 64
      state.last_v_xor_length += !state.last_v_xor_length * 64;

      state.last_v_xor_trailing_z = 64 - v_xor_leading_z - state.last_v_xor_length;
    }

    const uint64_t v_xor = reader.consume_bits_u64(state.last_v_xor_length) << state.last_v_xor_trailing_z;
    const uint64_t v = std::bit_cast<uint64_t>(state.last_v) ^ v_xor;
    state.last_v = std::bit_cast<double>(v);
  }
};

template <TimestampDecoderInterface TimestampDecoder, ValuesDecoderInterface ValuesDecoder>
class StreamDecoder;

template <TimestampEncoderInterface TimestampEncoder, ValuesEncoderInterface ValuesEncoder>
class PROMPP_ATTRIBUTE_PACKED StreamEncoder {
  EncoderState state_;

  template <TimestampDecoderInterface TimestampDecoder, ValuesDecoderInterface ValuesDecoder>
  friend class StreamDecoder;

 public:
  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t last_timestamp() const noexcept { return state_.timestamp_encoder.last_ts; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE double last_value() const noexcept { return state_.values_encoder.last_v; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const EncoderState& state() const noexcept { return state_; }

  template <class BitSequence>
  inline __attribute__((always_inline)) void encode(int64_t ts, double v, BitSequence& ts_bitseq, BitSequence& v_bitseq) noexcept {
    if (state_.state == GorillaState::kFirstPoint) {
      TimestampEncoder::encode(state_.timestamp_encoder, ts, ts_bitseq);
      ValuesEncoder::encode_first(state_.values_encoder, v, v_bitseq);

      state_.state = GorillaState::kSecondPoint;
    } else if (state_.state == GorillaState::kSecondPoint) {
      TimestampEncoder::encode_delta(state_.timestamp_encoder, ts, ts_bitseq);
      ValuesEncoder::encode(state_.values_encoder, v, v_bitseq);

      state_.state = GorillaState::kOtherPoint;
    } else {
      TimestampEncoder::encode_delta_of_delta(state_.timestamp_encoder, ts, ts_bitseq);
      ValuesEncoder::encode(state_.values_encoder, v, v_bitseq);
    }
  }

  PROMPP_ALWAYS_INLINE void reset(const DecoderState& state) noexcept {
    state_.timestamp_encoder = state.timestamp_decoder;
    state_.values_encoder.last_v = state.values_decoder.last_v;
    state_.values_encoder.last_v_xor_length = state.values_decoder.last_v_xor_length;
    state_.values_encoder.last_v_xor_trailing_z = state.values_decoder.last_v_xor_trailing_z;
    state_.values_encoder.v_xor_waste_bits_written = 0;
    state_.state = static_cast<decltype(state_.state)>(state.state);
  }
};

template <TimestampEncoderInterface TimestampEncoder, TimestampDecoderInterface TimestampDecoder>
class PROMPP_ATTRIBUTE_PACKED PrometheusStreamEncoder {
 public:
  PROMPP_ALWAYS_INLINE void encode(int64_t timestamp, double value, BitSequence& stream) {
    if (timestamp > encoder_.last_timestamp()) {
      append(timestamp, value, stream);
      return;
    } else if (timestamp == encoder_.last_timestamp() && value == encoder_.last_value()) {
      return;
    }

    insert(timestamp, value, stream);
  }

 private:
  StreamEncoder<TimestampEncoder, ValuesEncoder> encoder_;

  PROMPP_ALWAYS_INLINE void append(int64_t timestamp, double value, BitSequence& stream) { encoder_.encode(timestamp, value, stream, stream); }

  PROMPP_ALWAYS_INLINE void insert(int64_t timestamp, double value, BitSequence& stream) {
    StreamDecoder<TimestampDecoder, ValuesDecoder> decoder;
    auto reader = stream.reader();
    auto [position, state] = decode_to_timestamp(timestamp, reader, decoder);
    if (timestamp == decoder.last_timestamp() && value == decoder.last_value()) {
      return;
    }

    encoder_.reset(state);

    BitSequence new_stream(stream, position);
    append(timestamp, value, new_stream);
    if (timestamp != decoder.last_timestamp()) {
      append(decoder.last_timestamp(), decoder.last_value(), new_stream);
    }

    re_encode(reader, decoder, new_stream);
    stream = std::move(new_stream);
  }

  PROMPP_ALWAYS_INLINE static std::pair<uint64_t, DecoderState> decode_to_timestamp(int64_t timestamp,
                                                                                    BitSequenceReader& reader,
                                                                                    StreamDecoder<TimestampDecoder, ValuesDecoder>& decoder) {
    std::pair<uint64_t, DecoderState> result;

    while (!reader.eof()) {
      result.first = reader.position();
      result.second = decoder.state();

      decoder.decode(reader, reader);
      if (decoder.last_timestamp() >= timestamp) {
        return result;
      }
    }

    assert(false);
    return result;
  }

  PROMPP_ALWAYS_INLINE void re_encode(BitSequenceReader& reader, StreamDecoder<TimestampDecoder, ValuesDecoder>& decoder, BitSequence& stream) {
    while (!reader.eof()) {
      decoder.decode(reader, reader);
      append(decoder.last_timestamp(), decoder.last_value(), stream);
    }
  }
};

static_assert(sizeof(StreamEncoder<ZigZagTimestampEncoder<>, ValuesEncoder>) == 29);
static_assert(sizeof(PrometheusStreamEncoder<TimestampEncoder, TimestampDecoder>) == 29);

template <TimestampDecoderInterface TimestampDecoder, ValuesDecoderInterface ValuesDecoder>
class PROMPP_ATTRIBUTE_PACKED StreamDecoder {
  DecoderState state_;

 public:
  StreamDecoder() = default;

  explicit inline __attribute__((always_inline)) StreamDecoder(const EncoderState& encoder_state) : state_{encoder_state} {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE const DecoderState& state() const noexcept { return state_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t last_timestamp() const noexcept { return state_.timestamp_decoder.last_ts; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE double last_value() const noexcept { return state_.values_decoder.last_v; }

  inline __attribute__((always_inline)) void decode(int64_t ts, BitSequenceReader& v_bitseq) noexcept {
    if (__builtin_expect(state_.state == GorillaState::kFirstPoint, false)) {
      state_.timestamp_decoder.last_ts = ts;

      ValuesDecoder::decode_first(state_.values_decoder, v_bitseq);

      state_.state = GorillaState::kSecondPoint;
    } else if (__builtin_expect(state_.state == GorillaState::kSecondPoint, false)) {
      state_.timestamp_decoder.last_ts_delta = std::bit_cast<int64_t>(ts) - std::bit_cast<int64_t>(state_.timestamp_decoder.last_ts);
      state_.timestamp_decoder.last_ts = ts;

      ValuesDecoder::decode(state_.values_decoder, v_bitseq);

      state_.state = GorillaState::kOtherPoint;
    } else {
      state_.timestamp_decoder.last_ts_delta = std::bit_cast<int64_t>(ts) - std::bit_cast<int64_t>(state_.timestamp_decoder.last_ts);
      state_.timestamp_decoder.last_ts = ts;

      ValuesDecoder::decode(state_.values_decoder, v_bitseq);
    }
  }

  inline __attribute__((always_inline)) void decode(BitSequenceReader& ts_bitseq, BitSequenceReader& v_bitseq) noexcept {
    if (__builtin_expect(state_.state == GorillaState::kFirstPoint, false)) {
      TimestampDecoder::decode(state_.timestamp_decoder, ts_bitseq);
      ValuesDecoder::decode_first(state_.values_decoder, v_bitseq);

      state_.state = GorillaState::kSecondPoint;
    } else if (__builtin_expect(state_.state == GorillaState::kSecondPoint, false)) {
      TimestampDecoder::decode_delta(state_.timestamp_decoder, ts_bitseq);
      ValuesDecoder::decode(state_.values_decoder, v_bitseq);

      state_.state = GorillaState::kOtherPoint;
    } else {
      TimestampDecoder::decode_delta_of_delta(state_.timestamp_decoder, ts_bitseq);
      ValuesDecoder::decode(state_.values_decoder, v_bitseq);
    }
  }

  template <class OutputStream>
  void save(OutputStream& out) {
    out.write(reinterpret_cast<const char*>(&state_), sizeof(state_));
  }

  template <class InputStream>
  void load(InputStream& in) {
    in.read(reinterpret_cast<char*>(&state_), sizeof(state_));
  }

  bool operator==(const StreamDecoder& decoder) const = default;
};

static_assert(sizeof(StreamDecoder<ZigZagTimestampDecoder<>, ValuesDecoder>) == 27);

}  // namespace Encoding::Gorilla

template <Encoding::Gorilla::TimestampEncoderInterface TimestampEncoder, Encoding::Gorilla::ValuesEncoderInterface ValuesEncoder>
struct IsTriviallyReallocatable<Encoding::Gorilla::StreamEncoder<TimestampEncoder, ValuesEncoder>> : std::true_type {};

template <Encoding::Gorilla::TimestampEncoderInterface TimestampEncoder, Encoding::Gorilla::ValuesEncoderInterface ValuesEncoder>
struct IsZeroInitializable<Encoding::Gorilla::StreamEncoder<TimestampEncoder, ValuesEncoder>> : std::true_type {};

template <Encoding::Gorilla::TimestampEncoderInterface TimestampEncoder, Encoding::Gorilla::ValuesEncoderInterface ValuesEncoder>
struct IsTriviallyDestructible<Encoding::Gorilla::StreamEncoder<TimestampEncoder, ValuesEncoder>> : std::true_type {};

template <Encoding::Gorilla::TimestampDecoderInterface TimestampDecoder>
struct IsTriviallyReallocatable<Encoding::Gorilla::StreamDecoder<TimestampDecoder, Encoding::Gorilla::ValuesDecoder>> : std::true_type {};

template <Encoding::Gorilla::TimestampDecoderInterface TimestampDecoder>
struct IsZeroInitializable<Encoding::Gorilla::StreamDecoder<TimestampDecoder, Encoding::Gorilla::ValuesDecoder>> : std::true_type {};

template <Encoding::Gorilla::TimestampDecoderInterface TimestampDecoder>
struct IsTriviallyDestructible<Encoding::Gorilla::StreamDecoder<TimestampDecoder, Encoding::Gorilla::ValuesDecoder>> : std::true_type {};

}  // namespace BareBones
