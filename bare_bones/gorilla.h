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

struct PROMPP_ATTRIBUTE_PACKED TimestampEncoderState {
  int64_t last_ts{};
  int64_t last_ts_delta{};  // for stream gorilla samples might not be ordered

  bool operator==(const TimestampEncoderState&) const noexcept = default;
};

template <class TimestampEncoder>
concept TimestampEncoderInterface = requires(TimestampEncoderState& state, BitSequence& sequence) {
  { TimestampEncoder::encode(state, int64_t(), sequence) };
  { TimestampEncoder::encode_delta(state, int64_t(), sequence) };
  { TimestampEncoder::encode_delta_of_delta(state, int64_t(), sequence) };
};

class ZigZagTimestampEncoder {
 public:
  PROMPP_ALWAYS_INLINE static void encode(TimestampEncoderState& state, int64_t ts, BitSequence& stream) {
    state.last_ts = ts;

    stream.push_back_u64_svbyte_0248(ZigZag::encode(ts));
  }

  PROMPP_ALWAYS_INLINE static void encode_delta(TimestampEncoderState& state, int64_t ts, BitSequence& stream) {
    state.last_ts_delta = ts - state.last_ts;
    state.last_ts = ts;

    stream.push_back_u64_svbyte_2468(ZigZag::encode(state.last_ts_delta));
  }

  PROMPP_ALWAYS_INLINE static void encode_delta_of_delta(TimestampEncoderState& state, int64_t ts, BitSequence& stream) {
    auto ts_delta = ts - state.last_ts;
    const int64_t delta_of_delta = ts_delta - state.last_ts_delta;
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

    state.last_ts_delta = ts_delta;
    state.last_ts = ts;
  }
};

static constexpr size_t kMaxVarintLength = 10;

struct PROMPP_ATTRIBUTE_PACKED ValuesEncoderState {
  double last_v{};
  uint8_t last_v_xor_length{};
  uint8_t last_v_xor_leading_z{};
  uint8_t last_v_xor_trailing_z{};
  uint8_t v_xor_waste_bits_written{};

  bool operator==(const ValuesEncoderState&) const noexcept = default;
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
  int64_t last_ts{};
  int64_t last_ts_delta{};  // for stream gorilla samples might not be ordered

  double last_v{};
  uint8_t last_v_xor_length{};
  uint8_t last_v_xor_trailing_z{};

  GorillaState state{GorillaState::kFirstPoint};

  DecoderState() = default;
  DecoderState(const DecoderState&) = default;
  DecoderState(DecoderState&&) noexcept = default;
  explicit DecoderState(const EncoderState& encoder_state)
      : last_ts(encoder_state.timestamp_encoder.last_ts),
        last_ts_delta(encoder_state.timestamp_encoder.last_ts_delta),
        last_v(encoder_state.values_encoder.last_v),
        last_v_xor_length(encoder_state.values_encoder.last_v_xor_length),
        last_v_xor_trailing_z(encoder_state.values_encoder.last_v_xor_trailing_z),
        state(static_cast<decltype(state)>(encoder_state.state)) {}

  DecoderState& operator=(const DecoderState&) = default;
  DecoderState& operator=(DecoderState&&) noexcept = default;

  bool operator==(const DecoderState&) const noexcept = default;
};

class TimestampEncoder {
 public:
  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode(TimestampEncoderState& state, int64_t ts, BitSequence& stream) {
    state.last_ts = ts;

    uint8_t varint_buffer[kMaxVarintLength]{};
    push_varint_buffer(varint_buffer, write_varint(varint_buffer, ts), stream);
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode_delta(TimestampEncoderState& state, int64_t ts, BitSequence& stream) {
    state.last_ts_delta = ts - state.last_ts;
    state.last_ts = ts;

    uint8_t varint_buffer[kMaxVarintLength]{};
    push_varint_buffer(varint_buffer, write_varint(varint_buffer, std::bit_cast<uint64_t>(state.last_ts_delta)), stream);
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

template <class ValuesEncoder>
concept ValuesEncoderInterface = requires(ValuesEncoder& encoder, BitSequence& sequence, ValuesEncoderState& state) {
  { encoder.encode_first(state, double(), sequence) };
  { encoder.encode(state, double(), sequence) };
};

class PROMPP_ATTRIBUTE_PACKED ValuesEncoder {
 public:
  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode_first(ValuesEncoderState& state, double v, BitSequence& stream) noexcept {
    state.last_v = v;

    stream.push_back_d64_svbyte_0468(v);
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode(ValuesEncoderState& state, double v, BitSequence& stream) noexcept {
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
};

template <TimestampDecoderInterface T>
class StreamDecoder;

template <TimestampEncoderInterface TimestampEncoder, ValuesEncoderInterface ValuesEncoder>
class PROMPP_ATTRIBUTE_PACKED StreamEncoder {
  EncoderState state_;

  template <TimestampDecoderInterface T>
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
    state_.timestamp_encoder.last_ts = state.last_ts;
    state_.timestamp_encoder.last_ts_delta = state.last_ts_delta;
    state_.values_encoder.last_v = state.last_v;
    state_.values_encoder.last_v_xor_length = state.last_v_xor_length;
    state_.values_encoder.last_v_xor_trailing_z = state.last_v_xor_trailing_z;
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
    StreamDecoder<TimestampDecoder> decoder;
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
                                                                                    BitSequence::Reader& reader,
                                                                                    StreamDecoder<TimestampDecoder>& decoder) {
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

  PROMPP_ALWAYS_INLINE void re_encode(BitSequence::Reader& reader, StreamDecoder<TimestampDecoder>& decoder, BitSequence& stream) {
    while (!reader.eof()) {
      decoder.decode(reader, reader);
      append(decoder.last_timestamp(), decoder.last_value(), stream);
    }
  }
};

static_assert(sizeof(StreamEncoder<ZigZagTimestampEncoder, ValuesEncoder>) == 29);
static_assert(sizeof(PrometheusStreamEncoder<TimestampEncoder, TimestampDecoder>) == 29);

template <TimestampDecoderInterface TimestampDecoder>
class PROMPP_ATTRIBUTE_PACKED StreamDecoder {
  DecoderState state_;

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
      state_.last_v_xor_length = Bit::bextr(buf, 7, 6);

      // if last_v_xor_length is zero it should be 64
      state_.last_v_xor_length += !state_.last_v_xor_length * 64;

      state_.last_v_xor_trailing_z = 64 - v_xor_leading_z - state_.last_v_xor_length;
    }

    const uint64_t v_xor = bitseq.consume_bits_u64(state_.last_v_xor_length) << state_.last_v_xor_trailing_z;
    const uint64_t v = std::bit_cast<uint64_t>(state_.last_v) ^ v_xor;
    state_.last_v = std::bit_cast<double>(v);
  }

 public:
  StreamDecoder() = default;

  explicit inline __attribute__((always_inline)) StreamDecoder(const EncoderState& encoder_state) : state_{encoder_state} {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE const DecoderState& state() const noexcept { return state_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t last_timestamp() const noexcept { return state_.last_ts; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE double last_value() const noexcept { return state_.last_v; }

  inline __attribute__((always_inline)) void decode(int64_t ts, BitSequence::Reader& v_bitseq) noexcept {
    if (__builtin_expect(state_.state == GorillaState::kFirstPoint, false)) {
      state_.last_ts = ts;

      state_.last_v = v_bitseq.consume_d64_svbyte_0468();

      state_.state = GorillaState::kSecondPoint;
    } else if (__builtin_expect(state_.state == GorillaState::kSecondPoint, false)) {
      state_.last_ts_delta = std::bit_cast<int64_t>(ts) - std::bit_cast<int64_t>(state_.last_ts);
      state_.last_ts = ts;

      decode_value(v_bitseq);

      state_.state = GorillaState::kOtherPoint;
    } else {
      state_.last_ts_delta = std::bit_cast<int64_t>(ts) - std::bit_cast<int64_t>(state_.last_ts);
      state_.last_ts = ts;

      decode_value(v_bitseq);
    }
  }

  inline __attribute__((always_inline)) void decode(BitSequence::Reader& ts_bitseq, BitSequence::Reader& v_bitseq) noexcept {
    if (__builtin_expect(state_.state == GorillaState::kFirstPoint, false)) {
      state_.last_ts = TimestampDecoder::decode(ts_bitseq);

      state_.last_v = v_bitseq.consume_d64_svbyte_0468();

      state_.state = GorillaState::kSecondPoint;
    } else if (__builtin_expect(state_.state == GorillaState::kSecondPoint, false)) {
      state_.last_ts_delta = TimestampDecoder::decode_delta(ts_bitseq);
      state_.last_ts += state_.last_ts_delta;

      decode_value(v_bitseq);

      state_.state = GorillaState::kOtherPoint;
    } else {
      uint32_t buf = ts_bitseq.read_u32();

      if (buf & 0b1) {
        state_.last_ts_delta += TimestampDecoder::decode_delta_of_delta(buf, ts_bitseq);
      } else {
        ts_bitseq.ff(1);
      }

      state_.last_ts += state_.last_ts_delta;

      decode_value(v_bitseq);
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

static_assert(sizeof(StreamDecoder<ZigZagTimestampDecoder>) == 27);

}  // namespace Encoding::Gorilla

template <Encoding::Gorilla::TimestampEncoderInterface TimestampEncoder, Encoding::Gorilla::ValuesEncoderInterface ValuesEncoder>
struct IsTriviallyReallocatable<Encoding::Gorilla::StreamEncoder<TimestampEncoder, ValuesEncoder>> : std::true_type {};

template <Encoding::Gorilla::TimestampEncoderInterface TimestampEncoder, Encoding::Gorilla::ValuesEncoderInterface ValuesEncoder>
struct IsZeroInitializable<Encoding::Gorilla::StreamEncoder<TimestampEncoder, ValuesEncoder>> : std::true_type {};

template <Encoding::Gorilla::TimestampEncoderInterface TimestampEncoder, Encoding::Gorilla::ValuesEncoderInterface ValuesEncoder>
struct IsTriviallyDestructible<Encoding::Gorilla::StreamEncoder<TimestampEncoder, ValuesEncoder>> : std::true_type {};

template <Encoding::Gorilla::TimestampDecoderInterface TimestampDecoder>
struct IsTriviallyReallocatable<Encoding::Gorilla::StreamDecoder<TimestampDecoder>> : std::true_type {};

template <Encoding::Gorilla::TimestampDecoderInterface TimestampDecoder>
struct IsZeroInitializable<Encoding::Gorilla::StreamDecoder<TimestampDecoder>> : std::true_type {};

template <Encoding::Gorilla::TimestampDecoderInterface TimestampDecoder>
struct IsTriviallyDestructible<Encoding::Gorilla::StreamDecoder<TimestampDecoder>> : std::true_type {};

}  // namespace BareBones
