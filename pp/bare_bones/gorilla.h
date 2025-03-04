#pragma once

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
concept TimestampEncoderInterface = requires(TimestampEncoder& encoder, const TimestampEncoder& const_encoder, BitSequence& sequence) {
  { const_encoder.timestamp() } -> std::same_as<int64_t>;
  { const_encoder.state };

  { encoder.encode(int64_t(), sequence) } -> std::same_as<void>;
  { encoder.encode_delta(int64_t(), sequence) } -> std::same_as<void>;
  { encoder.encode_delta_of_delta(int64_t(), sequence) } -> std::same_as<void>;
};

struct DodSignificantLengths {
  uint8_t first;
  uint8_t second;
  uint8_t third;
};

static constexpr auto kDefaultDodSignificantLengths = DodSignificantLengths{.first = 5, .second = 15, .third = 18};

template <DodSignificantLengths kDogSignificantLengths = kDefaultDodSignificantLengths>
class PROMPP_ATTRIBUTE_PACKED ZigZagTimestampEncoder {
 public:
  TimestampEncoderState state{};

  ZigZagTimestampEncoder() noexcept = default;
  explicit ZigZagTimestampEncoder(const TimestampEncoderState& state) noexcept : state(state) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t timestamp() const noexcept { return state.last_ts; }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE void encode(int64_t ts, BitSequence& stream) {
    state.last_ts = ts;

    stream.push_back_u64_svbyte_0248(ZigZag::encode(ts));
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE void encode_delta(int64_t ts, BitSequence& stream) {
    state.last_ts_delta = ts - state.last_ts;
    state.last_ts = ts;

    stream.push_back_u64_svbyte_2468(ZigZag::encode(state.last_ts_delta));
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE void encode_delta_of_delta(int64_t ts, BitSequence& stream) {
    const auto ts_delta = ts - state.last_ts;
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
      const uint8_t ts_dod_significant_len = 64 - std::countl_zero(ts_dod_zigzag);

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
  PROMPP_ALWAYS_INLINE void encode_delta_of_delta_with_stale_nan(double timestamp, BitSequence& stream) {
    if (isstalenan(timestamp)) [[unlikely]] {
      stream.push_back_bits_u32(5, 0b11111);
    } else {
      const auto ts = static_cast<int64_t>(timestamp);
      const auto ts_delta = ts - state.last_ts;
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
      const uint8_t ts_dod_significant_len = 64 - std::countl_zero(ts_dod_zigzag);

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

struct PROMPP_ATTRIBUTE_PACKED ValuesDecoderState {
  double last_v{};
  uint8_t last_v_xor_length{};
  uint8_t last_v_xor_trailing_z{};

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint8_t last_v_xor_leading_z() const noexcept {
    return Bit::to_bits(sizeof(uint64_t)) - last_v_xor_trailing_z - last_v_xor_length;
  }

  bool operator==(const ValuesDecoderState&) const noexcept = default;
};

struct PROMPP_ATTRIBUTE_PACKED ValuesEncoderState {
  double last_v{};
  uint8_t last_v_xor_length{};
  uint8_t last_v_xor_leading_z{Bit::to_bits(sizeof(uint64_t))};
  uint8_t last_v_xor_trailing_z{};
  uint8_t v_xor_waste_bits_written{};

  ValuesEncoderState() = default;
  explicit ValuesEncoderState(const ValuesDecoderState& state) noexcept
      : last_v(state.last_v),
        last_v_xor_length(state.last_v_xor_length),
        last_v_xor_leading_z(state.last_v_xor_leading_z()),
        last_v_xor_trailing_z(state.last_v_xor_trailing_z) {}

  bool operator==(const ValuesEncoderState&) const noexcept = default;

  PROMPP_ALWAYS_INLINE void reset() noexcept { reset({}); }

  PROMPP_ALWAYS_INLINE void reset(const ValuesDecoderState& state) noexcept {
    this->last_v = state.last_v;
    this->last_v_xor_length = state.last_v_xor_length;
    this->last_v_xor_leading_z = state.last_v_xor_leading_z();
    this->last_v_xor_trailing_z = state.last_v_xor_trailing_z;
    this->v_xor_waste_bits_written = {};
  }
};

enum class GorillaState : uint8_t {
  kFirstPoint = 0,
  kSecondPoint = 1,
  kOtherPoint = 2,
};

class PROMPP_ATTRIBUTE_PACKED TimestampEncoder {
 public:
  TimestampEncoderState state{};

  TimestampEncoder() noexcept = default;
  explicit TimestampEncoder(const TimestampEncoderState& state) noexcept : state(state) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t timestamp() const noexcept { return state.last_ts; }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE void encode(int64_t ts, BitSequence& stream) {
    state.last_ts = ts;

    uint8_t varint_buffer[VarInt::kMaxVarIntLength]{};
    push_varint_buffer(varint_buffer, VarInt::write(varint_buffer, ts), stream);
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE void encode_delta(int64_t ts, BitSequence& stream) {
    state.last_ts_delta = ts - state.last_ts;
    state.last_ts = ts;

    uint8_t varint_buffer[VarInt::kMaxVarIntLength]{};
    push_varint_buffer(varint_buffer, VarInt::write(varint_buffer, std::bit_cast<uint64_t>(state.last_ts_delta)), stream);
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE void encode_delta_of_delta(int64_t ts, BitSequence& stream) {
    const auto ts_delta = ts - state.last_ts;
    const int64_t delta_of_delta = ts_delta - state.last_ts_delta;

    if (const auto ts_dod_zigzag = std::bit_cast<uint64_t>(delta_of_delta); ts_dod_zigzag == 0) {
      stream.push_back_single_zero_bit();
    } else {
      const uint8_t ts_dod_significant_len = 64 - std::countl_zero(ts_dod_zigzag);

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
concept TimestampDecoderInterface = requires(TimestampDecoder& decoder, const TimestampDecoder& const_decoder, BitSequenceReader& reader, int64_t timestamp) {
  { const_decoder.timestamp() } -> std::same_as<int64_t>;
  { const_decoder.state() };

  { const_decoder == const_decoder } -> std::same_as<bool>;

  { decoder.decode(reader) } -> std::same_as<void>;
  { decoder.decode(timestamp) } -> std::same_as<void>;

  { decoder.decode_delta(reader) } -> std::same_as<void>;
  { decoder.decode_delta(timestamp) } -> std::same_as<void>;

  { decoder.decode_delta_of_delta(reader) } -> std::same_as<void>;
  { decoder.decode_delta_of_delta(timestamp) } -> std::same_as<void>;
};

template <DodSignificantLengths kDogSignificantLengths = kDefaultDodSignificantLengths>
class ZigZagTimestampNullDecoder {
 public:
  [[nodiscard]] PROMPP_ALWAYS_INLINE static int64_t timestamp() noexcept {
    assert(false);

    return 0;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE auto& state() const noexcept {
    assert(false);

    static struct {
    } state;
    return state;
  }

  PROMPP_ALWAYS_INLINE static void decode(int64_t) noexcept {}
  PROMPP_ALWAYS_INLINE static void decode(BitSequenceReader& reader) { reader.ff_u64_svbyte_0248(); }

  PROMPP_ALWAYS_INLINE static void decode_delta(int64_t) noexcept {}
  PROMPP_ALWAYS_INLINE static void decode_delta(BitSequenceReader& reader) { reader.ff_u64_svbyte_2468(); }

  PROMPP_ALWAYS_INLINE static void decode_delta_of_delta(int64_t) noexcept {}
  PROMPP_ALWAYS_INLINE static void decode_delta_of_delta(BitSequenceReader& reader) {
    if (const uint32_t buf = reader.read_u32(); buf & 0b1) {
      if ((buf & 0b10) == 0) {
        reader.ff(2 + kDogSignificantLengths.first);
      } else if ((buf & 0b100) == 0) {
        reader.ff(3 + kDogSignificantLengths.second);
      } else if ((buf & 0b1000) == 0) {
        reader.ff(4 + kDogSignificantLengths.third);
      } else {
        reader.ff(4);
        reader.ff_u64_svbyte_2468();
      }
    } else {
      reader.ff(1);
    }
  }

  bool operator==(const ZigZagTimestampNullDecoder& other) const noexcept = default;
};

template <DodSignificantLengths kDogSignificantLengths = kDefaultDodSignificantLengths>
class PROMPP_ATTRIBUTE_PACKED ZigZagTimestampDecoder {
 public:
  ZigZagTimestampDecoder() = default;
  explicit ZigZagTimestampDecoder(const TimestampEncoderState& state) : state_(state) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t timestamp() const noexcept { return state_.last_ts; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const TimestampEncoderState& state() const noexcept { return state_; }

  PROMPP_ALWAYS_INLINE void decode(int64_t timestamp) noexcept { state_.last_ts = timestamp; }
  PROMPP_ALWAYS_INLINE void decode(BitSequenceReader& reader) { state_.last_ts = ZigZag::decode(reader.consume_u64_svbyte_0248()); }

  PROMPP_ALWAYS_INLINE void decode_delta(int64_t timestamp) noexcept {
    state_.last_ts_delta = timestamp - state_.last_ts;
    state_.last_ts = timestamp;
  }
  PROMPP_ALWAYS_INLINE void decode_delta(BitSequenceReader& reader) {
    state_.last_ts_delta = ZigZag::decode(reader.consume_u64_svbyte_2468());
    state_.last_ts += state_.last_ts_delta;
  }

  PROMPP_ALWAYS_INLINE void decode_delta_of_delta(int64_t timestamp) noexcept {
    state_.last_ts_delta = timestamp - state_.last_ts;
    state_.last_ts = timestamp;
  }
  PROMPP_ALWAYS_INLINE void decode_delta_of_delta(BitSequenceReader& reader) {
    if (const uint32_t buf = reader.read_u32(); buf & 0b1) {
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

      state_.last_ts_delta += ZigZag::decode(dod_zigzag);
    } else {
      reader.ff(1);
    }

    state_.last_ts += state_.last_ts_delta;
  }

  PROMPP_ALWAYS_INLINE ValueType decode_delta_of_delta_with_stale_nan(BitSequenceReader& reader) {
    if (const uint32_t buf = reader.read_u32(); buf & 0b1) {
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

      state_.last_ts_delta += ZigZag::decode(dod_zigzag);
    } else {
      reader.ff(1);
    }

    state_.last_ts += state_.last_ts_delta;
    return ValueType::kValue;
  }

  bool operator==(const ZigZagTimestampDecoder& other) const noexcept = default;

 private:
  TimestampEncoderState state_{};
};

class PROMPP_ATTRIBUTE_PACKED TimestampDecoder {
 public:
  TimestampDecoder() = default;
  explicit TimestampDecoder(const TimestampEncoderState& state) : state_(state) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t timestamp() const noexcept { return state_.last_ts; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const TimestampEncoderState& state() const noexcept { return state_; }

  PROMPP_ALWAYS_INLINE void decode(int64_t timestamp) noexcept { state_.last_ts = timestamp; }
  PROMPP_ALWAYS_INLINE void decode(BitSequenceReader& reader) { state_.last_ts = VarInt::read(reader); }

  PROMPP_ALWAYS_INLINE void decode_delta(int64_t timestamp) noexcept {
    state_.last_ts_delta = timestamp - state_.last_ts;
    state_.last_ts = timestamp;
  }
  PROMPP_ALWAYS_INLINE void decode_delta(BitSequenceReader& reader) {
    state_.last_ts_delta = std::bit_cast<int64_t>(VarInt::read_uint(reader));
    state_.last_ts += state_.last_ts_delta;
  }

  PROMPP_ALWAYS_INLINE void decode_delta_of_delta(int64_t timestamp) noexcept {
    state_.last_ts_delta = timestamp - state_.last_ts;
    state_.last_ts = timestamp;
  }
  void decode_delta_of_delta(BitSequenceReader& reader) {
    if (const uint32_t buf = reader.read_u32(); buf & 0b1) {
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

      state_.last_ts_delta += std::bit_cast<int64_t>(dod_zigzag);
    } else {
      reader.ff(1);
    }

    state_.last_ts += state_.last_ts_delta;
  }

  bool operator==(const TimestampDecoder& other) const noexcept = default;

 private:
  TimestampEncoderState state_{};
};

template <class ValuesEncoder>
concept ValuesEncoderInterface = requires(ValuesEncoder& encoder, const ValuesEncoder& const_encoder, BitSequence& sequence) {
  { encoder.value() } -> std::same_as<double>;
  { encoder.state() };

  { encoder.encode_first(double(), sequence) } -> std::same_as<void>;
  { encoder.encode(double(), sequence) } -> std::same_as<void>;
};

class PROMPP_ATTRIBUTE_PACKED ValuesEncoder {
 public:
  ValuesEncoder() noexcept = default;
  explicit ValuesEncoder(const ValuesDecoderState& state) noexcept : state_(state) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE double value() const noexcept { return state_.last_v; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const ValuesEncoderState& state() const noexcept { return state_; }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE void encode_first(double v, BitSequence& stream) noexcept {
    state_.last_v = v;

    stream.push_back_d64_svbyte_0468(v);
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE void encode_first(double v, uint32_t count, BitSequence& stream) noexcept {
    assert(count >= 1);

    encode_first(v, stream);
    stream.push_back_single_zero_bit(count - 1);
  }

  template <class BitSequence>
  void encode(double v, BitSequence& stream) noexcept {
    const uint64_t v_xor = std::bit_cast<uint64_t>(state_.last_v) ^ std::bit_cast<uint64_t>(v);

    state_.last_v = v;

    if (v_xor == 0) {
      stream.push_back_single_zero_bit();
      return;
    }

    uint8_t v_xor_leading_z = std::countl_zero(v_xor);
    const uint8_t v_xor_trailing_z = std::countr_zero(v_xor);

    // we store lead_z in 5bits in encoding, so it's limited by 31
    v_xor_leading_z = v_xor_leading_z > 31 ? 31 : v_xor_leading_z;

    const uint8_t v_xor_length = 64 - v_xor_leading_z - v_xor_trailing_z;

    // we need to write xor length, if it was never written
    if (state_.last_v_xor_length == 0)
      goto write_xor_length;

    // we need to write xor length, if xor doesn't fit into the same bit range
    if (v_xor_leading_z < state_.last_v_xor_leading_z || v_xor_trailing_z < state_.last_v_xor_trailing_z)
      goto write_xor_length;

    // heuristics that optimizes gorilla size based on one-time length change or amount of unnecessary bits written
    {
      // always positive, because we already checked that xor fits into the same bit range
      const uint8_t v_xor_length_delta = state_.last_v_xor_length - v_xor_length;

      // we need to write xor length
      //  * either because of accumulated statistics (more than 50 waste bits were written since last xor length write)
      //  * or because of one time drastic change (length is smaller for more than 11 bits)
      if (state_.v_xor_waste_bits_written >= 50 || v_xor_length_delta >= 11)
        goto write_xor_length;

      // we zero waste bits if length difference is less than 3
      state_.v_xor_waste_bits_written = v_xor_length_delta < 3 ? 0 : state_.v_xor_waste_bits_written;

      // count unnecessary bits
      state_.v_xor_waste_bits_written += v_xor_length_delta;
    }

    // if we got here we don't need to write xor length
    stream.push_back_bits_u32(2, 0b01);

    stream.push_back_bits_u64(state_.last_v_xor_length, v_xor >> state_.last_v_xor_trailing_z);
    return;

  write_xor_length:
    state_.v_xor_waste_bits_written = 0;
    state_.last_v_xor_length = v_xor_length;
    state_.last_v_xor_leading_z = v_xor_leading_z;
    state_.last_v_xor_trailing_z = v_xor_trailing_z;
    assert(state_.last_v_xor_length + state_.last_v_xor_trailing_z <= 64);

    stream.push_back_bits_u32(1 + 1 + 5 + 6, 0b11 | (v_xor_leading_z << (1 + 1)) | (v_xor_length << (1 + 1 + 5)));
    stream.push_back_bits_u64(state_.last_v_xor_length, v_xor >> state_.last_v_xor_trailing_z);
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE void encode(double v, uint32_t count, BitSequence& stream) noexcept {
    assert(count >= 1);

    encode(v, stream);
    stream.push_back_single_zero_bit(count - 1);
  }

 private:
  ValuesEncoderState state_{};
};

template <class ValuesDecoder>
concept ValuesDecoderInterface = requires(ValuesDecoder& decoder, const ValuesDecoder& const_decoder, BitSequenceReader& reader) {
  { const_decoder.value() } -> std::same_as<double>;
  { const_decoder.state() };

  { const_decoder == const_decoder } -> std::same_as<bool>;

  { decoder.decode_first(reader) } -> std::same_as<void>;
  { decoder.decode(reader) } -> std::same_as<void>;
};

class PROMPP_ATTRIBUTE_PACKED ValuesNullDecoder {
 public:
  [[nodiscard]] PROMPP_ALWAYS_INLINE static double value() noexcept {
    assert(false);

    return 0.0;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const auto& state() const noexcept {
    assert(false);

    return state_;
  }

  PROMPP_ALWAYS_INLINE static void decode_first(BitSequenceReader& reader) noexcept { reader.ff_d64_svbyte_0468(); }

  void decode(BitSequenceReader& reader) noexcept {
    const uint32_t buf = reader.read_u32();

    if ((buf & 0b1) == 0) {
      reader.ff(1);
      return;
    }

    reader.ff(1 + 1);

    // length changed?
    if (buf & 0b10) {
      reader.ff(5 + 6);

      state_.last_v_xor_length = Bit::bextr(buf, 7, 6);
      state_.last_v_xor_length += !state_.last_v_xor_length * 64;
    }

    reader.ff(state_.last_v_xor_length);
  }

  bool operator==(const ValuesNullDecoder& other) const noexcept = default;

 private:
  struct PROMPP_ATTRIBUTE_PACKED State {
    uint8_t last_v_xor_length{};

    bool operator==(const State& other) const noexcept = default;
  };

  State state_{};
};

class PROMPP_ATTRIBUTE_PACKED ValuesDecoder {
 public:
  ValuesDecoder() = default;
  explicit ValuesDecoder(const ValuesEncoderState& state)
      : state_{.last_v = state.last_v, .last_v_xor_length = state.last_v_xor_length, .last_v_xor_trailing_z = state.last_v_xor_trailing_z} {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE double value() const noexcept { return state_.last_v; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const ValuesDecoderState& state() const noexcept { return state_; }

  PROMPP_ALWAYS_INLINE void decode_first(BitSequenceReader& reader) noexcept { state_.last_v = reader.consume_d64_svbyte_0468(); }

  void decode(BitSequenceReader& reader) noexcept {
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

      const uint8_t v_xor_leading_z = Bit::bextr(buf, 2, 5);
      state_.last_v_xor_length = Bit::bextr(buf, 7, 6);

      // if last_v_xor_length is zero it should be 64
      state_.last_v_xor_length += !state_.last_v_xor_length * 64;

      state_.last_v_xor_trailing_z = 64 - v_xor_leading_z - state_.last_v_xor_length;
    }

    const uint64_t v_xor = reader.consume_bits_u64(state_.last_v_xor_length) << state_.last_v_xor_trailing_z;
    const uint64_t v = std::bit_cast<uint64_t>(state_.last_v) ^ v_xor;
    state_.last_v = std::bit_cast<double>(v);
  }

  bool operator==(const ValuesDecoder& other) const noexcept = default;

 private:
  ValuesDecoderState state_{};
};

template <TimestampDecoderInterface TimestampDecoder, ValuesDecoderInterface ValuesDecoder>
class StreamDecoder;

template <TimestampEncoderInterface TimestampEncoder, ValuesEncoderInterface ValuesEncoder>
class PROMPP_ATTRIBUTE_PACKED StreamEncoder {
  struct PROMPP_ATTRIBUTE_PACKED State {
    TimestampEncoder timestamp_encoder;
    ValuesEncoder values_encoder;
    GorillaState state{GorillaState::kFirstPoint};

    State() = default;
    template <class DecoderState>
    explicit State(const DecoderState& state) noexcept
        : timestamp_encoder(state.timestamp_decoder.state()), values_encoder(state.values_decoder.state()), state(state.state) {}

    bool operator==(const State&) const noexcept = default;
  };

  State state_;

  template <TimestampDecoderInterface TimestampDecoder, ValuesDecoderInterface ValuesDecoder>
  friend class StreamDecoder;

 public:
  StreamEncoder() = default;
  template <class DecoderState>
  explicit StreamEncoder(const DecoderState& state) : state_(state) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t last_timestamp() const noexcept { return state_.timestamp_encoder.timestamp(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE double last_value() const noexcept { return state_.values_encoder.value(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const State& state() const noexcept { return state_; }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE void encode(int64_t ts, double v, BitSequence& ts_bitseq, BitSequence& v_bitseq) noexcept {
    if (state_.state == GorillaState::kFirstPoint) [[unlikely]] {
      state_.timestamp_encoder.encode(ts, ts_bitseq);
      state_.values_encoder.encode_first(v, v_bitseq);

      state_.state = GorillaState::kSecondPoint;
    } else if (state_.state == GorillaState::kSecondPoint) [[unlikely]] {
      state_.timestamp_encoder.encode_delta(ts, ts_bitseq);
      state_.values_encoder.encode(v, v_bitseq);

      state_.state = GorillaState::kOtherPoint;
    } else {
      state_.timestamp_encoder.encode_delta_of_delta(ts, ts_bitseq);
      state_.values_encoder.encode(v, v_bitseq);
    }
  }
};

static_assert(sizeof(StreamEncoder<ZigZagTimestampEncoder<>, ValuesEncoder>) == 29);

template <TimestampDecoderInterface TimestampDecoder, ValuesDecoderInterface ValuesDecoder>
class PROMPP_ATTRIBUTE_PACKED StreamDecoder {
  struct PROMPP_ATTRIBUTE_PACKED State {
    [[no_unique_address]] TimestampDecoder timestamp_decoder;
    [[no_unique_address]] ValuesDecoder values_decoder;
    GorillaState state{GorillaState::kFirstPoint};

    State() = default;
    State(const State&) = default;
    State(State&&) noexcept = default;

    template <class EncoderState>
    explicit State(const EncoderState& state)
        : timestamp_decoder(state.timestamp_encoder.state), values_decoder{state.values_encoder.state()}, state(state.state) {}

    State& operator=(const State&) = default;
    State& operator=(State&&) noexcept = default;

    bool operator==(const State&) const noexcept = default;
  };

  State state_;

 public:
  StreamDecoder() = default;

  template <class EncoderState>
  explicit PROMPP_ALWAYS_INLINE StreamDecoder(const EncoderState& state) : state_{state} {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE const State& state() const noexcept { return state_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t last_timestamp() const noexcept { return state_.timestamp_decoder.timestamp(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE double last_value() const noexcept { return state_.values_decoder.value(); }

  PROMPP_ALWAYS_INLINE void decode(int64_t ts, BitSequenceReader& v_bitseq) noexcept {
    if (state_.state == GorillaState::kFirstPoint) [[unlikely]] {
      state_.timestamp_decoder.decode(ts);
      state_.values_decoder.decode_first(v_bitseq);

      state_.state = GorillaState::kSecondPoint;
    } else if (state_.state == GorillaState::kSecondPoint) [[unlikely]] {
      state_.timestamp_decoder.decode_delta(ts);
      state_.values_decoder.decode(v_bitseq);

      state_.state = GorillaState::kOtherPoint;
    } else {
      state_.timestamp_decoder.decode_delta_of_delta(ts);
      state_.values_decoder.decode(v_bitseq);
    }
  }

  PROMPP_ALWAYS_INLINE void decode(BitSequenceReader& ts_bitseq, BitSequenceReader& v_bitseq) noexcept {
    if (state_.state == GorillaState::kFirstPoint) [[unlikely]] {
      state_.timestamp_decoder.decode(ts_bitseq);
      state_.values_decoder.decode_first(v_bitseq);

      state_.state = GorillaState::kSecondPoint;
    } else if (state_.state == GorillaState::kSecondPoint) [[unlikely]] {
      state_.timestamp_decoder.decode_delta(ts_bitseq);
      state_.values_decoder.decode(v_bitseq);

      state_.state = GorillaState::kOtherPoint;
    } else {
      state_.timestamp_decoder.decode_delta_of_delta(ts_bitseq);
      state_.values_decoder.decode(v_bitseq);
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
static_assert(sizeof(StreamDecoder<ZigZagTimestampNullDecoder<>, ValuesNullDecoder>) == 2);

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
