#pragma once

#include "parallel_hashmap/phmap.h"

#include "bare_bones/allocator.h"
#include "bare_bones/gorilla.h"
#include "bare_bones/vector_with_holes.h"
#include "series_data/encoder/bit_sequence.h"

namespace series_data::encoder::timestamp {

struct PROMPP_ATTRIBUTE_PACKED State {
  using Id = uint32_t;

  static constexpr auto kInvalidId = std::numeric_limits<Id>::max();

  BareBones::Encoding::Gorilla::TimestampEncoderState encoder_state;
  union PROMPP_ATTRIBUTE_PACKED StreamData {
    ~StreamData() {}

    void destruct_stream() { stream.~BitSequenceWithItemsCount(); }

    BitSequenceWithItemsCount stream;
    uint32_t finalized_stream_id;
  } stream_data{.stream{}};
  uint32_t reference_count{1};
  Id previous_state_id{kInvalidId};

  explicit State(Id previous_id) : previous_state_id(previous_id) {}
  State(const State& state, Id previous_id)
      : encoder_state(state.encoder_state), stream_data{.stream = state.stream_data.stream}, previous_state_id(previous_id) {}
  State(State&& other) noexcept : encoder_state(other.encoder_state), previous_state_id(other.previous_state_id) {
    if (!other.is_finalized()) {
      [[likely]];
      stream_data.stream = std::move(other.stream_data.stream);
    } else {
      stream_data.finalized_stream_id = other.stream_data.finalized_stream_id;
    }
  }
  ~State() {
    if (!is_finalized()) {
      [[likely]];
      stream_data.destruct_stream();
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_finalized() const noexcept { return encoder_state.last_ts_delta == kFinalizedState; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t timestamp() const noexcept { return encoder_state.last_ts; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return is_finalized() ? 0 : stream_data.stream.allocated_memory(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE BitSequenceWithItemsCount finalize(uint32_t finalized_stream_id) noexcept {
    assert(!is_finalized());

    auto result = std::move(stream_data.stream);
    result.stream.shrink_to_fit();
    stream_data.finalized_stream_id = finalized_stream_id;
    encoder_state.last_ts_delta = kFinalizedState;
    return result;
  }

 private:
  static constexpr auto kFinalizedState = std::numeric_limits<int64_t>::min();
};

class StateTransitions {
 public:
  using Hash = uint64_t;
  struct Key {
    const State::Id previous_state_id;
    const State::Id state_id;

    Key(State::Id previous, State::Id id) : previous_state_id(previous), state_id(id) {}

    PROMPP_ALWAYS_INLINE bool operator==(const Key&) const noexcept = default;
  };

  explicit StateTransitions(const BareBones::VectorWithHoles<State>& states)
      : state_transitions_{0, HashCalculator{states}, EqualTo{states}, BareBones::Allocator<Key>{state_transitions_allocated_memory_}} {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE static uint64_t hash(int64_t timestamp, State::Id state_id) noexcept {
    return phmap::phmap_mix<sizeof(size_t)>()(HashCalculator::hash(timestamp, state_id));
  }

  PROMPP_ALWAYS_INLINE void emplace(Hash hash, State::Id previous_state_id, State::Id state_id) noexcept {
    state_transitions_.emplace_with_hash(hash, previous_state_id, state_id);
  }
  [[nodiscard]] const Key* get(Hash hash, int64_t timestamp, State::Id state_id) const noexcept {
    if (auto it = state_transitions_.find(TimestampKey{.timestamp = timestamp, .state_id = state_id}, hash); it != state_transitions_.end()) {
      return &*it;
    }

    return nullptr;
  }
  PROMPP_ALWAYS_INLINE void erase(const State& state) noexcept { state_transitions_.erase(timestamp_key(state)); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return state_transitions_allocated_memory_; }

 private:
  struct PROMPP_ATTRIBUTE_PACKED TimestampKey {
    const int64_t timestamp;
    const State::Id state_id;

    bool operator==(const TimestampKey&) const noexcept = default;
  };

  class HashCalculator {
   public:
    using is_transparent = void;

    explicit HashCalculator(const BareBones::VectorWithHoles<State>& states) : states_(states) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE static size_t hash(int64_t timestamp, State::Id state_id) noexcept {
      return std::bit_cast<uint64_t>(timestamp) | (static_cast<uint64_t>(state_id) << 44);
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t operator()(const TimestampKey& key) const noexcept { return hash(key.timestamp, key.state_id); }
    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t operator()(const Key& key) const noexcept {
      return hash(states_[key.state_id].encoder_state.last_ts, key.previous_state_id);
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

      return states_[a.state_id].encoder_state.last_ts == b.timestamp;
    }

   private:
    const BareBones::VectorWithHoles<State>& states_;
  };

  size_t state_transitions_allocated_memory_{};
  phmap::flat_hash_set<Key, HashCalculator, EqualTo, BareBones::Allocator<Key>> state_transitions_;

  [[nodiscard]] PROMPP_ALWAYS_INLINE static TimestampKey timestamp_key(const State& state) noexcept {
    return {.timestamp = state.encoder_state.last_ts, .state_id = state.previous_state_id};
  }
};

}  // namespace series_data::encoder::timestamp

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::timestamp::State> : std::true_type {};
