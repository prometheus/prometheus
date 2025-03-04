#pragma once

#include "state.h"

namespace series_data::encoder::timestamp {

class TimestampEncoder {
 public:
  static void encode_first(State::TimestampEncoder& encoder, int64_t timestamp, BitSequenceWithItemsCount& stream) { encoder.encode(timestamp, stream.stream); }

  static void encode(State::TimestampEncoder& encoder, int64_t timestamp, BitSequenceWithItemsCount& stream) {
    if (stream.inc_count() == 1) [[unlikely]] {
      encoder.encode_delta(timestamp, stream.stream);
    } else {
      encoder.encode_delta_of_delta(timestamp, stream.stream);
    }
  }
};

class TimestampDecoder {
 public:
  explicit TimestampDecoder(const BareBones::BitSequenceReader& reader) : reader_(reader) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t decode() noexcept {
    if (gorilla_state_ == GorillaState::kFirstPoint) [[unlikely]] {
      decoder_.decode(reader_);
      gorilla_state_ = GorillaState::kSecondPoint;
    } else if (gorilla_state_ == GorillaState::kSecondPoint) [[unlikely]] {
      decoder_.decode_delta(reader_);
      gorilla_state_ = GorillaState::kOtherPoint;
    } else {
      decoder_.decode_delta_of_delta(reader_);
    }

    return decoder_.timestamp();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool eof() const noexcept { return reader_.eof(); }

  [[nodiscard]] static int64_t decode_first(BareBones::BitSequenceReader reader) noexcept {
    State::TimestampDecoder decoder;
    decoder.decode(reader);
    return decoder.timestamp();
  }

  [[nodiscard]] static BareBones::Vector<int64_t> decode_all(const BareBones::BitSequenceReader& reader, uint8_t count) noexcept {
    BareBones::Vector<int64_t> values;

    TimestampDecoder decoder(reader);
    for (uint8_t i = 0; i < count; ++i) {
      values.emplace_back(decoder.decode());
    }

    return values;
  }

 private:
  using GorillaState = BareBones::Encoding::Gorilla::GorillaState;

  BareBones::BitSequenceReader reader_;
  State::TimestampDecoder decoder_;
  GorillaState gorilla_state_{GorillaState::kFirstPoint};
};

class Encoder {
 public:
  State::Id encode(State::Id state_id, int64_t timestamp) {
    const auto hash = StateTransitions::hash(timestamp, state_id);

    if (const auto transition = state_transitions_.get(hash, timestamp, state_id); transition != nullptr) {
      const auto new_state_id = transition->state_id;
      if (state_id != State::kInvalidId) {
        decrease_reference_count(states_[state_id], state_id);
      }

      ++states_[new_state_id].reference_count;
      return new_state_id;
    }

    const auto previous_state_id = state_id;
    if (state_id == State::kInvalidId) [[unlikely]] {
      auto& state = states_.emplace_back(state_id);
      TimestampEncoder::encode_first(state.encoder, timestamp, state.stream_data.stream);
      state_id = states_.index_of(state);
    } else {
      auto& new_state = states_.emplace_back(state_id);

      auto& state = states_[state_id];
      ++state.child_count;

      if (state.reference_count > 1) [[likely]] {
        new_state = state;
      } else {
        new_state = std::move(state);
      }

      decrease_reference_count(state, state_id);
      state_id = states_.index_of(new_state);

      TimestampEncoder::encode(new_state.encoder, timestamp, new_state.stream_data.stream);
    }

    state_transitions_.emplace(hash, previous_state_id, state_id);
    return state_id;
  }

  PROMPP_ALWAYS_INLINE void erase(State::Id state_id) { decrease_reference_count(states_[state_id], state_id); }

  PROMPP_ALWAYS_INLINE void finalize_or_copy(State::Id state_id, BitSequenceWithItemsCount& stream, uint32_t finalized_stream_id) {
    if (auto& state = states_[state_id]; --state.reference_count == 0) {
      stream = state.finalize(finalized_stream_id);

      state_transitions_.erase(state);
      decrease_previous_state_child_count(state_id, state.previous_state_id);
      if (state.child_count == 0) {
        states_.erase(state_id);
      }
    } else {
      stream = state.stream_data.stream;
      stream.stream.shrink_to_fit();
    }
  }

  PROMPP_ALWAYS_INLINE void finalize(State::Id state_id, BitSequenceWithItemsCount& stream, uint32_t finalized_stream_id) {
    auto& state = states_[state_id];
    stream = state.finalize(finalized_stream_id);
    decrease_reference_count(state, state_id);
  }

  PROMPP_ALWAYS_INLINE uint32_t process_finalized(State::Id state_id) {
    if (auto& state = states_[state_id]; state.is_finalized()) [[unlikely]] {
      const auto result = state.stream_data.finalized_stream_id;
      decrease_reference_count(state, state_id);
      return result;
    }

    return State::kInvalidId;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return states_.allocated_memory() + state_transitions_.allocated_memory(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const BitSequenceWithItemsCount& get_stream(State::Id state_id) const noexcept {
    return states_[state_id].stream_data.stream;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE State& get_state(State::Id state_id) noexcept { return states_[state_id]; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const State& get_state(State::Id state_id) const noexcept { return states_[state_id]; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_unique_state(State::Id state_id) const noexcept {
    auto& state = states_[state_id];
    return state.reference_count == 1 && state.child_count == 0;
  }

 private:
  BareBones::VectorWithHoles<State> states_;
  StateTransitions state_transitions_{states_};

  PROMPP_ALWAYS_INLINE void decrease_reference_count(State& state, State::Id state_id) noexcept {
    if (--state.reference_count == 0) {
      state_transitions_.erase(state);
      decrease_previous_state_child_count(state_id, state.previous_state_id);
      if (state.child_count == 0) {
        states_.erase(state_id);
      } else {
        state.free_memory();
      }
    }
  }

  PROMPP_ALWAYS_INLINE void decrease_previous_state_child_count(uint32_t state_id, uint32_t previous_state_id) noexcept {
    while (previous_state_id != State::kInvalidId) {
      states_[state_id].previous_state_id = State::kInvalidId;
      auto& previous_state = states_[previous_state_id];

      assert(previous_state.child_count > 0);

      if (--previous_state.child_count == 0 && previous_state.reference_count == 0) {
        state_id = previous_state_id;
        previous_state_id = previous_state.previous_state_id;

        states_.erase(state_id);
        continue;
      }

      return;
    }
  }
};

}  // namespace series_data::encoder::timestamp