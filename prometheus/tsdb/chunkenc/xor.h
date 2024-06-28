#pragma once

#include <bitset>

#include "bare_bones/gorilla.h"

namespace PromPP::Prometheus::tsdb::chunkenc {

class TimestampEncoder {
 public:
  template <class BStream>
  PROMPP_ALWAYS_INLINE static void encode(BareBones::Encoding::Gorilla::TimestampEncoderState& state, int64_t ts, BStream& stream) {
    state.last_ts = ts;

    uint8_t varint_buffer[VarInt::kMaxVarIntLength]{};
    push_varint_buffer(varint_buffer, VarInt::write(varint_buffer, ts), stream);
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode_delta(BareBones::Encoding::Gorilla::TimestampEncoderState& state, int64_t ts, BitSequence& stream) {
    state.last_ts_delta = ts - state.last_ts;
    state.last_ts = ts;

    uint8_t varint_buffer[VarInt::kMaxVarIntLength]{};
    push_varint_buffer(varint_buffer, VarInt::write(varint_buffer, std::bit_cast<uint64_t>(state.last_ts_delta)), stream);
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode_delta_of_delta(BareBones::Encoding::Gorilla::TimestampEncoderState& state, int64_t ts, BitSequence& stream) {
    static constexpr uint8_t kDogSignificantLengths[] = {14, 17, 20};

    auto ts_delta = ts - state.last_ts;
    const int64_t dod = ts_delta - state.last_ts_delta;

    if (dod == 0) {
      stream.write_zero_bit();
    } else if (bit_range(dod, kDogSignificantLengths[0])) {
      stream.write_bits((0b10 << kDogSignificantLengths[0]) | (std::bit_cast<uint64_t>(dod) & get_bit_mask(kDogSignificantLengths[0])),
                        2 + kDogSignificantLengths[0]);
    } else if (bit_range(dod, kDogSignificantLengths[1])) {
      stream.write_bits((0b110 << kDogSignificantLengths[1]) | (std::bit_cast<uint64_t>(dod) & get_bit_mask(kDogSignificantLengths[1])),
                        3 + kDogSignificantLengths[1]);
    } else if (bit_range(dod, kDogSignificantLengths[2])) {
      stream.write_bits((0b1110 << kDogSignificantLengths[2]) | (std::bit_cast<uint64_t>(dod) & get_bit_mask(kDogSignificantLengths[2])),
                        4 + kDogSignificantLengths[2]);
    } else {
      stream.write_bits(0b1111, 4);
      stream.write_bits(std::bit_cast<uint64_t>(dod), BareBones::Bit::kUint64Bits);
    }

    state.last_ts_delta = ts_delta;
    state.last_ts = ts;
  }

 private:
  using VarInt = BareBones::Encoding::VarInt;

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void push_varint_buffer(const uint8_t* buffer, size_t bytes, BitSequence& stream) {
    for (auto b = buffer, end = buffer + bytes; b != end; ++b) {
      stream.write_byte(*b);
    }
  }

  PROMPP_ALWAYS_INLINE constexpr static bool bit_range(int64_t x, uint8_t nbits) noexcept { return -((1 << (nbits - 1)) - 1) <= x && x <= 1 << (nbits - 1); }

  PROMPP_ALWAYS_INLINE constexpr static uint64_t get_bit_mask(uint8_t bits) noexcept {
    return std::numeric_limits<uint64_t>::max() >> (BareBones::Bit::kUint64Bits - bits);
  }
};

class ValuesEncoder {
 public:
  template <class BStream>
  PROMPP_ALWAYS_INLINE static void encode_first(BareBones::Encoding::Gorilla::ValuesEncoderState& state, double v, BStream& stream) noexcept {
    state.last_v = v;

    stream.write_bits(std::bit_cast<uint64_t>(v), BareBones::Bit::kUint64Bits);
  }

  template <class BitSequence>
  static void encode(BareBones::Encoding::Gorilla::ValuesEncoderState& state, double v, BitSequence& stream) noexcept {
    uint64_t v_xor = std::bit_cast<uint64_t>(state.last_v) ^ std::bit_cast<uint64_t>(v);

    state.last_v = v;

    if (v_xor == 0) {
      stream.write_zero_bit();
      return;
    }

    uint8_t v_xor_leading_z = std::countl_zero(v_xor);
    uint8_t v_xor_trailing_z = std::countr_zero(v_xor);

    // we store lead_z in 5bits in encoding, so it's limited by 31
    v_xor_leading_z = v_xor_leading_z > 31 ? 31 : v_xor_leading_z;

    uint8_t v_xor_length = BareBones::Bit::kUint64Bits - v_xor_leading_z - v_xor_trailing_z;

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
      if (state.v_xor_waste_bits_written >= 50 || v_xor_length_delta >= 11) {
        goto write_xor_length;
      }

      // we zero waste bits if length difference is less than 3
      state.v_xor_waste_bits_written = v_xor_length_delta < 3 ? 0 : state.v_xor_waste_bits_written;

      // count unnecessary bits
      state.v_xor_waste_bits_written += v_xor_length_delta;
    }

    // if we got here we don't need to write xor length
    stream.write_bits(0b10, 2);
    stream.write_bits(v_xor >> state.last_v_xor_trailing_z, state.last_v_xor_length);
    return;

  write_xor_length:
    state.v_xor_waste_bits_written = 0;
    state.last_v_xor_length = v_xor_length;
    state.last_v_xor_leading_z = v_xor_leading_z;
    state.last_v_xor_trailing_z = v_xor_trailing_z;
    assert(state.last_v_xor_length + state.last_v_xor_trailing_z <= BareBones::Bit::to_bits(sizeof(uint64_t)));

    stream.write_bits((0b11 << (5 + 6)) | (v_xor_leading_z << 6) | v_xor_length, 1 + 1 + 5 + 6);
    stream.write_bits(v_xor >> state.last_v_xor_trailing_z, state.last_v_xor_length);
  }
};

}  // namespace PromPP::Prometheus::tsdb::chunkenc