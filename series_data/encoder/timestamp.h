#pragma once

#include "parallel_hashmap/phmap.h"

#include "allocation_sizes_table.h"
#include "bare_bones/allocator.h"
#include "bare_bones/gorilla.h"
#include "bare_bones/vector_with_holes.h"

namespace series_data::encoder::timestamp {

class PROMPP_ATTRIBUTE_PACKED TimestampEncoder {
 public:
  explicit TimestampEncoder(int64_t timestamp) { BareBones::Encoding::Gorilla::TimestampEncoder::encode(encoder_state_, timestamp, stream_); }

  void encode(int64_t timestamp) {
    if (count_ == 1) {
      [[unlikely]];
      BareBones::Encoding::Gorilla::TimestampEncoder::encode_delta(encoder_state_, timestamp, stream_);
    } else {
      BareBones::Encoding::Gorilla::TimestampEncoder::encode_delta_of_delta(encoder_state_, timestamp, stream_);
    }

    ++count_;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return stream_.allocated_memory(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t count() const noexcept { return count_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t last_timestamp() const noexcept { return encoder_state_.last_ts; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE BareBones::BitSequenceReader reader() const noexcept { return stream_.reader(); }

 private:
  BareBones::Encoding::Gorilla::TimestampEncoderState encoder_state_;
  BareBones::CompactBitSequence<kAllocationSizesTable> stream_;
  uint32_t count_{1};
};

using StateId = uint32_t;
constexpr auto kInvalidStateId = std::numeric_limits<StateId>::max();

struct Key {
  uint32_t previous_state_id;
  uint32_t state_id;

  Key(uint32_t _previous_state_id, uint32_t _state_id) : previous_state_id(_previous_state_id), state_id(_state_id) {}

  PROMPP_ALWAYS_INLINE bool operator==(const Key&) const noexcept = default;
};

struct PROMPP_ATTRIBUTE_PACKED TimestampKey {
  int64_t timestamp;
  uint32_t state_id;

  bool operator==(const TimestampKey&) const noexcept = default;
};

struct PROMPP_ATTRIBUTE_PACKED State {
  TimestampEncoder encoder;
  uint32_t reference_count{1};
  StateId previous_state_id{kInvalidStateId};

  explicit State(int64_t timestamp, StateId _previous_state_id) : encoder(timestamp), previous_state_id(_previous_state_id) {}
  explicit State(TimestampEncoder encoder, StateId _previous_state_id) : encoder(std::move(encoder)), previous_state_id(_previous_state_id) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE TimestampKey timestamp_key() const noexcept {
    return {.timestamp = encoder.last_timestamp(), .state_id = previous_state_id};
  }

  PROMPP_ALWAYS_INLINE void encode(int64_t timestamp) { encoder.encode(timestamp); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return encoder.allocated_memory(); }
};

class Hash {
 public:
  using is_transparent = void;

  explicit Hash(const BareBones::VectorWithHoles<State>& states) : states_(states) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE static size_t hash(int64_t timestamp, StateId state_id) noexcept {
    return std::bit_cast<uint64_t>(timestamp) | (static_cast<uint64_t>(state_id) << 44);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t operator()(const TimestampKey& key) const noexcept { return hash(key.timestamp, key.state_id); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t operator()(const Key& key) const noexcept {
    return hash(states_[key.state_id].encoder.last_timestamp(), key.previous_state_id);
  }

 private:
  const BareBones::VectorWithHoles<State>& states_;
};

class EqualTo {
 public:
  using is_transparent = void;

  explicit EqualTo(const BareBones::VectorWithHoles<State>& states) : states_(states) {}

  PROMPP_ALWAYS_INLINE bool operator()(const Key& a, const Key& b) const noexcept { return a == b; }
  PROMPP_ALWAYS_INLINE bool operator()(const Key& a, const TimestampKey& b) const noexcept {
    if (a.previous_state_id != b.state_id) {
      return false;
    }

    return states_[a.state_id].encoder.last_timestamp() == b.timestamp;
  }

 private:
  const BareBones::VectorWithHoles<State>& states_;
};

}  // namespace series_data::encoder::timestamp

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::timestamp::State> : std::true_type {};

namespace series_data::encoder::timestamp {

class Encoder {
 public:
  StateId encode(StateId state_id, int64_t timestamp) {
    auto hash = phmap::phmap_mix<sizeof(size_t)>()(Hash::hash(timestamp, state_id));

    auto it = state_transitions_.find(TimestampKey{.timestamp = timestamp, .state_id = state_id}, hash);
    if (it != state_transitions_.end()) {
      auto new_state_id = it->state_id;
      if (state_id != kInvalidStateId) {
        auto& old_value = states_[state_id];
        if (--old_value.reference_count == 0) {
          state_transitions_.erase(old_value.timestamp_key());
          states_.erase(state_id);
        }
      }

      ++states_[new_state_id].reference_count;
      return new_state_id;
    }

    auto previous_state_id = state_id;
    if (state_id == kInvalidStateId) {
      [[unlikely]];
      state_id = states_.index_of(states_.emplace_back(timestamp, state_id));
    } else {
      auto& old_value = states_[state_id];
      if (old_value.reference_count > 1) {
        --old_value.reference_count;

        auto& value = states_.emplace_back(old_value.encoder, state_id);
        value.encode(timestamp);

        state_id = states_.index_of(value);
      } else {
        state_transitions_.erase(old_value.timestamp_key());

        old_value.previous_state_id = state_id;
        old_value.encode(timestamp);
      }
    }

    state_transitions_.emplace_with_hash(hash, previous_state_id, state_id);
    return state_id;
  }

  PROMPP_ALWAYS_INLINE void erase(StateId state_id) {
    auto& state = states_[state_id];
    if (--state.reference_count == 0) {
      state_transitions_.erase(state.timestamp_key());
      states_.erase(state_id);
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return states_allocated_memory_ + states_.allocated_memory(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const TimestampEncoder& get_encoder(StateId state_id) const noexcept { return states_[state_id].encoder; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE TimestampEncoder& get_encoder(StateId state_id) noexcept { return states_[state_id].encoder; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_unique_state(StateId state_id) noexcept { return states_[state_id].previous_state_id == state_id; }

 public:
  BareBones::VectorWithHoles<State> states_;
  size_t states_allocated_memory_{};
  phmap::flat_hash_set<Key, Hash, EqualTo, BareBones::Allocator<Key>> state_transitions_{0, Hash{states_}, EqualTo{states_},
                                                                                         BareBones::Allocator<Key>{states_allocated_memory_}};
};

}  // namespace series_data::encoder::timestamp