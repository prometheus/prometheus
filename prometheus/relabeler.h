#pragma once

#include <cstdint>
#include <cstring>
#include <sstream>
#include <string>
#include <string_view>

#include <parallel_hashmap/phmap.h>
#include <roaring/roaring.hh>

#include "bare_bones/allocator.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/vector.h"
#include "hashdex.h"
#include "primitives/go_slice.h"
#include "primitives/labels_builder.h"
#include "primitives/sample.h"
#include "primitives/timeseries.h"
#include "stateless_relabeler.h"
#include "value.h"

namespace PromPP::Prometheus::Relabel {

// MetricLimits limits on label set and samples.
struct MetricLimits {
  size_t label_limit{0};
  size_t label_name_length_limit{0};
  size_t label_value_length_limit{0};
  size_t sample_limit{0};

  PROMPP_ALWAYS_INLINE bool label_limit_exceeded(size_t labels_count) { return label_limit > 0 && labels_count > label_limit; }

  PROMPP_ALWAYS_INLINE bool samples_limit_exceeded(size_t samples_count) { return sample_limit > 0 && samples_count >= sample_limit; }
};

// hard_validate on empty, name label(__name__) mandatory, valid label name and value) validate label set.
template <class LabelsBuilder>
PROMPP_ALWAYS_INLINE void hard_validate(relabelStatus& rstatus, LabelsBuilder& builder, MetricLimits* limits) {
  if (rstatus == rsDrop) {
    return;
  }

  // check on empty labels set
  if (builder.is_empty()) [[unlikely]] {
    rstatus = rsDrop;
    return;
  }

  // check on contains metric name labels set
  if (!builder.contains(kMetricLabelName)) [[unlikely]] {
    rstatus = rsInvalid;
    return;
  }

  // validate labels
  builder.range([&]<typename LNameType, typename LValueType>(LNameType& lname, LValueType& lvalue) PROMPP_LAMBDA_INLINE -> bool {
    if (lname == kMetricLabelName && !metric_name_value_is_valid(lvalue)) {
      rstatus = rsInvalid;
      return false;
    }

    if (!label_name_is_valid(lname) || !label_value_is_valid(lvalue)) {
      rstatus = rsInvalid;
      return false;
    }

    return true;
  });
  if (rstatus == rsInvalid) [[unlikely]] {
    return;
  }

  if (limits == nullptr) {
    return;
  }

  // check limit len serie
  if (limits->label_limit_exceeded(builder.size())) {
    rstatus = rsInvalid;
    return;
  }

  if (limits->label_name_length_limit == 0 && limits->label_value_length_limit == 0) {
    return;
  }

  // check limit len label name and value
  builder.range([&]<typename LNameType, typename LValueType>(LNameType& lname, LValueType& lvalue) PROMPP_LAMBDA_INLINE -> bool {
    if (limits->label_name_length_limit > 0 && lname.size() > limits->label_name_length_limit) {
      rstatus = rsInvalid;
      return false;
    }

    if (limits->label_value_length_limit > 0 && lvalue.size() > limits->label_value_length_limit) {
      rstatus = rsInvalid;
      return false;
    }

    return true;
  });
};

// InnerSerie - timeserie after relabeling.
//
// samples - incoming samples;
// ls_id   - relabeling ls id from lss;
struct InnerSerie {
  BareBones::Vector<PromPP::Primitives::Sample> samples;
  uint32_t ls_id;

  PROMPP_ALWAYS_INLINE bool operator==(const InnerSerie& rt) const noexcept = default;
};

// InnerSeries - vector with relabeled result.
//
// size - number of timeseries processed;
// data - vector with timeseries;
class InnerSeries {
  size_t size_{0};
  std::vector<InnerSerie> data_;

 public:
  PROMPP_ALWAYS_INLINE const std::vector<InnerSerie>& data() const { return data_; }

  PROMPP_ALWAYS_INLINE size_t size() const { return size_; }

  PROMPP_ALWAYS_INLINE void reserve(size_t n) { data_.reserve(n); }

  PROMPP_ALWAYS_INLINE void emplace_back(const BareBones::Vector<PromPP::Primitives::Sample>& samples, const uint32_t& ls_id) {
    data_.emplace_back(samples, ls_id);
    ++size_;
  }

  PROMPP_ALWAYS_INLINE void clear() noexcept {
    data_.clear();
    size_ = 0;
  }
};

// RelabeledSerie - element after relabeling with new ls(for next step).
//
// ls      - relabeling new label set;
// samples - incoming samples;
// hash    - hash sum from ls;
// ls_id   - incoming ls id from lss;
struct RelabeledSerie {
  PromPP::Primitives::LabelSet ls;
  BareBones::Vector<PromPP::Primitives::Sample> samples;
  size_t hash;
  uint32_t ls_id;
};

// RelabeledSeries - vector with relabeling elements.
//
// size - number of timeseries processed;
// data - vector with RelabelElement;
class RelabeledSeries {
  size_t size_{0};
  std::vector<RelabeledSerie> data_;

 public:
  PROMPP_ALWAYS_INLINE const std::vector<RelabeledSerie>& data() const { return data_; }

  PROMPP_ALWAYS_INLINE size_t size() const { return size_; }

  PROMPP_ALWAYS_INLINE void emplace_back(PromPP::Primitives::LabelSet& ls,
                                         const BareBones::Vector<PromPP::Primitives::Sample>& samples,
                                         const size_t hash,
                                         const uint32_t ls_id) {
    data_.emplace_back(ls, samples, hash, ls_id);
    ++size_;
  }
};

// CacheValue - value for cache map.
//
// ls_id    - relabeled ls id;
// shard_id - relabeled shard id;
struct PROMPP_ATTRIBUTE_PACKED CacheValue {
  uint32_t ls_id{};
  uint16_t shard_id{};
};

// IncomingAndRelabeledLsID - for update cache.
struct IncomingAndRelabeledLsID {
  uint32_t incoming_ls_id{};
  uint32_t relabeled_ls_id{};
};

// RelabelerStateUpdate - container for update states.
class RelabelerStateUpdate {
  std::vector<IncomingAndRelabeledLsID> data_;

 public:
  PROMPP_ALWAYS_INLINE explicit RelabelerStateUpdate() {}

  PROMPP_ALWAYS_INLINE const std::vector<IncomingAndRelabeledLsID>& data() const { return data_; }

  PROMPP_ALWAYS_INLINE void emplace_back(const uint32_t incoming_ls_id, uint32_t relabeled_ls_id) { data_.emplace_back(incoming_ls_id, relabeled_ls_id); }

  PROMPP_ALWAYS_INLINE size_t size() const { return data_.size(); }

  PROMPP_ALWAYS_INLINE const IncomingAndRelabeledLsID& operator[](uint32_t i) const {
    assert(i < data_.size());
    return data_[i];
  }
};

class NoOpStaleNaNsState {
 public:
  PROMPP_ALWAYS_INLINE void add_input([[maybe_unused]] uint32_t id) {}
  PROMPP_ALWAYS_INLINE void add_target([[maybe_unused]] uint32_t id) {}

  template <typename InputCallback, typename TargetCallback>
  PROMPP_ALWAYS_INLINE void swap([[maybe_unused]] InputCallback input_fn, [[maybe_unused]] TargetCallback target_fn) {}
};

// StaleNaNsState state for stale nans.
class StaleNaNsState {
  roaring::Roaring input_bitset_{};
  roaring::Roaring target_bitset_{};
  roaring::Roaring prev_input_bitset_{};
  roaring::Roaring prev_target_bitset_{};

 public:
  PROMPP_ALWAYS_INLINE explicit StaleNaNsState() {}

  PROMPP_ALWAYS_INLINE void add_input(uint32_t id) { input_bitset_.add(id); }

  PROMPP_ALWAYS_INLINE void add_target(uint32_t id) { target_bitset_.add(id); }

  template <typename InputCallback, typename TargetCallback>
  PROMPP_ALWAYS_INLINE void swap(InputCallback input_fn, TargetCallback target_fn) {
    prev_input_bitset_ -= input_bitset_;
    for (uint32_t ls_id : prev_input_bitset_) {
      input_fn(ls_id);
    }
    // drop old, store new..
    prev_input_bitset_ = std::move(input_bitset_);

    prev_target_bitset_ -= target_bitset_;
    for (uint32_t ls_id : prev_target_bitset_) {
      target_fn(ls_id);
    }
    // drop old, store new..
    prev_target_bitset_ = std::move(target_bitset_);
  }

  PROMPP_ALWAYS_INLINE void reset() {
    input_bitset_ = roaring::Roaring{};
    target_bitset_ = roaring::Roaring{};
    prev_input_bitset_ = roaring::Roaring{};
    prev_target_bitset_ = roaring::Roaring{};
  }
};

// Cache stateless cache for relabeler.
class Cache {
  size_t cache_allocated_memory_{0};
  // phmap::btree_map<uint32_t,
  //                  PromPP::Prometheus::Relabel::CacheValue,
  //                  std::less<>,
  //                  BareBones::Allocator<std::pair<const uint32_t, PromPP::Prometheus::Relabel::CacheValue>>>
  //     cache_relabel_{{}, {}, BareBones::Allocator<std::pair<const uint32_t, PromPP::Prometheus::Relabel::CacheValue>>{cache_allocated_memory_}};
  phmap::flat_hash_map<uint32_t,
                       PromPP::Prometheus::Relabel::CacheValue,
                       std::hash<uint32_t>,
                       std::equal_to<>,
                       BareBones::Allocator<std::pair<const uint32_t, PromPP::Prometheus::Relabel::CacheValue>>>
      cache_relabel_{{}, {}, BareBones::Allocator<std::pair<const uint32_t, PromPP::Prometheus::Relabel::CacheValue>>{cache_allocated_memory_}};
  roaring::Roaring cache_keep_{};
  roaring::Roaring cache_drop_{};

 public:
  PROMPP_ALWAYS_INLINE explicit Cache() {}

  // allocated_memory return size of allocated memory for caches.
  PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return cache_allocated_memory_ + cache_keep_.getSizeInBytes() + cache_drop_.getSizeInBytes();
  }

  // add_drop add ls id to drop cache.
  PROMPP_ALWAYS_INLINE void add_drop(const uint32_t ls_id) { cache_drop_.add(ls_id); }

  // add_keep add ls id to keep cache.
  PROMPP_ALWAYS_INLINE void add_keep(const uint32_t ls_id) { cache_keep_.add(ls_id); }

  // add_relabel add ls id to relabel cache.
  PROMPP_ALWAYS_INLINE void add_relabel(const uint32_t ls_id, const uint32_t relabeled_ls_id, const uint16_t relabeled_shard_id) noexcept {
    cache_relabel_.emplace(ls_id, CacheValue{.ls_id = relabeled_ls_id, .shard_id = relabeled_shard_id});
  }

  // run optimization on bitset caches.
  PROMPP_ALWAYS_INLINE void optimize() {
    cache_keep_.runOptimize();
    cache_drop_.runOptimize();
  }

  PROMPP_ALWAYS_INLINE void reset() {
    cache_relabel_.clear();
    cache_keep_ = roaring::Roaring{};
    cache_drop_ = roaring::Roaring{};
  }

  PROMPP_ALWAYS_INLINE double part_of_drops() {
    if (cache_drop_.cardinality() == 0) {
      return 0;
    }

    return std::bit_cast<double>(cache_drop_.cardinality()) /
           std::bit_cast<double>(cache_drop_.cardinality() + cache_keep_.cardinality() + static_cast<uint64_t>(cache_relabel_.size()));
  }

  struct CheckResult {
    enum Status : uint8_t {
      kNotFound = 0,
      kDrop = 1,
      kKeep = 2,
      kRelabel = 3,
    };
    Status status{Status::kNotFound};
    uint16_t shard_id{};  // used only for kRelabel status
    uint32_t ls_id{};
    uint32_t source_ls_id{};  // used only for kRelabel status
  };

  template <class InputLSS, class TargetLSS, class LabelSet>
  PROMPP_ALWAYS_INLINE CheckResult check(const InputLSS& input_lss, const TargetLSS& target_lss, LabelSet& label_set, size_t hash) {
    if (std::optional<uint32_t> ls_id = input_lss.find(label_set, hash); ls_id.has_value()) {
      auto res = check_input(ls_id.value());
      if (res.status != CheckResult::kNotFound) {
        return res;
      }
    }
    if (std::optional<uint32_t> ls_id = target_lss.find(label_set, hash); ls_id.has_value()) {
      return check_target(ls_id.value());
    }
    return {};
  }

  PROMPP_ALWAYS_INLINE CheckResult check_input(uint32_t ls_id) {
    if (cache_drop_.contains(ls_id)) {
      return {.status = CheckResult::Status::kDrop};
    }

    if (auto it = cache_relabel_.find(ls_id); it != cache_relabel_.end()) {
      return {.status = CheckResult::Status::kRelabel, .shard_id = it->second.shard_id, .ls_id = it->second.ls_id, .source_ls_id = ls_id};
    }

    return {};
  }

  PROMPP_ALWAYS_INLINE CheckResult check_target(uint32_t ls_id) {
    if (cache_keep_.contains(ls_id)) {
      return {.status = CheckResult::Status::kKeep, .ls_id = ls_id};
    }

    return {};
  }
};

struct RelabelerOptions {
  PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> target_labels{};
  MetricLimits* metric_limits{nullptr};
  bool honor_labels{false};
  bool track_timestamps_staleness{false};
  bool honor_timestamps{false};
};

// PerShardRelabeler - relabeler for shard.
//
// buf_                 - stringstream for construct pattern part;
// builder_state_       - state of label set builder;
// timeseries_buf_      - buffer for read incoming timeseries;
// stateless_relabeler_ - shared stateless relabeler, pointer;
// shard_id_            - current shard id;
// log_shards_          - logarithm to the base 2 of total shards count;
class PerShardRelabeler {
  std::stringstream buf_;
  PromPP::Primitives::LabelsBuilderStateMap builder_state_;
  std::vector<PromPP::Primitives::LabelView> external_labels_{};
  PromPP::Primitives::TimeseriesSemiview timeseries_buf_;
  StatelessRelabeler* stateless_relabeler_;
  uint16_t number_of_shards_;
  uint16_t shard_id_;

 public:
  // PerShardRelabeler - constructor. Init only with pre-initialized LSS* and StatelessRelabeler*.
  PROMPP_ALWAYS_INLINE PerShardRelabeler(
      PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>>& external_labels,
      StatelessRelabeler* stateless_relabeler,
      const uint16_t number_of_shards,
      const uint16_t shard_id)
      : stateless_relabeler_(stateless_relabeler), number_of_shards_(number_of_shards), shard_id_(shard_id) {
    if (stateless_relabeler_ == nullptr) [[unlikely]] {
      throw BareBones::Exception(0xabd6db40882fd6aa, "stateless relabeler is null pointer");
    }

    external_labels_.reserve(external_labels.size());
    for (const auto& [ln, lv] : external_labels) {
      external_labels_.emplace_back(static_cast<std::string_view>(ln), static_cast<std::string_view>(lv));
    }
  }

  // TODO delete after rebuild metrics
  // cache_allocated_memory - return size of allocated memory for cache map.
  PROMPP_ALWAYS_INLINE size_t cache_allocated_memory() const noexcept { return 0; }

 private:
  PROMPP_ALWAYS_INLINE bool resolve_timestamps(PromPP::Primitives::Timestamp def_timestamp,
                                               BareBones::Vector<PromPP::Primitives::Sample>& samples,
                                               const RelabelerOptions& o) {
    // skip resolve without stalenans
    if (def_timestamp == PromPP::Primitives::kNullTimestamp) {
      return false;
    }

    bool track_staleness{true};
    for (auto& sample : samples) {
      // replace null timestamp on def timestamp
      if (sample.timestamp() == PromPP::Primitives::kNullTimestamp) {
        sample.timestamp() = def_timestamp;
        continue;
      }

      // replace incoming timestamp on def timestamp
      if (!o.honor_timestamps) {
        sample.timestamp() = def_timestamp;
        continue;
      }

      track_staleness = false;
    }

    return track_staleness;
  }

  template <class InputLSS, class TargetLSS, hashdex::HashdexInterface Hashdex, class StNaNsState, class Stats>
  PROMPP_ALWAYS_INLINE void input_relabeling_internal(InputLSS& input_lss,
                                                      TargetLSS& target_lss,
                                                      Cache& cache,
                                                      const Hashdex& hashdex,
                                                      const RelabelerOptions& o,
                                                      Stats& stats,
                                                      PromPP::Primitives::Go::SliceView<InnerSeries*>& shards_inner_series,
                                                      PromPP::Primitives::Go::SliceView<RelabeledSeries*>& shards_relabeled_series,
                                                      StNaNsState& stale_nan_state,
                                                      PromPP::Primitives::Timestamp def_timestamp) {
    assert(number_of_shards_ > 0);

    size_t n = std::min(static_cast<size_t>(hashdex.size()), static_cast<size_t>((hashdex.size() * (1 - cache.part_of_drops()) * 1.1) / number_of_shards_));
    for (auto i = 0; i < number_of_shards_; ++i) {
      shards_inner_series[i]->reserve(n);
    }

    PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state_};
    size_t samples_count{0};

    for (const auto& item : hashdex) {
      if ((item.hash() % number_of_shards_) != shard_id_) {
        continue;
      }

      timeseries_buf_.clear();
      item.read(timeseries_buf_);

      Cache::CheckResult check_result = cache.check(input_lss, target_lss, timeseries_buf_.label_set(), item.hash());
      switch (check_result.status) {
        case Cache::CheckResult::kNotFound: {
          builder.reset(timeseries_buf_.label_set());
          auto rstatus = relabel(o, builder);
          switch (rstatus) {
            case rsDrop: {
              cache.add_drop(input_lss.find_or_emplace(timeseries_buf_.label_set(), item.hash()));
              continue;
            }
            case rsInvalid: {
              cache.add_drop(input_lss.find_or_emplace(timeseries_buf_.label_set(), item.hash()));
              continue;
            }
            case rsKeep: {
              auto ls_id = target_lss.find_or_emplace(timeseries_buf_.label_set(), item.hash());
              cache.add_keep(ls_id);
              auto& samples = timeseries_buf_.samples();
              if (o.track_timestamps_staleness || resolve_timestamps(def_timestamp, samples, o)) {
                stale_nan_state.add_target(ls_id);
              }
              shards_inner_series[shard_id_]->emplace_back(samples, ls_id);
              ++stats.series_added;
            } break;
            case rsRelabel: {
              auto ls_id = input_lss.find_or_emplace(timeseries_buf_.label_set(), item.hash());
              PromPP::Primitives::LabelSet new_label_set = builder.label_set();
              size_t new_hash = hash_value(new_label_set);
              size_t new_shard_id = new_hash % number_of_shards_;
              auto& samples = timeseries_buf_.samples();
              if (o.track_timestamps_staleness || resolve_timestamps(def_timestamp, samples, o)) {
                stale_nan_state.add_input(ls_id);
              }
              shards_relabeled_series[new_shard_id]->emplace_back(new_label_set, samples, new_hash, ls_id);
              ++stats.series_added;
            } break;
          }
        } break;
        case Cache::CheckResult::kKeep: {
          auto& samples = timeseries_buf_.samples();
          if (o.track_timestamps_staleness || resolve_timestamps(def_timestamp, samples, o)) {
            stale_nan_state.add_target(check_result.ls_id);
          }
          shards_inner_series[shard_id_]->emplace_back(samples, check_result.ls_id);
        } break;
        case Cache::CheckResult::kRelabel: {
          auto& samples = timeseries_buf_.samples();
          if (o.track_timestamps_staleness || resolve_timestamps(def_timestamp, samples, o)) {
            stale_nan_state.add_input(check_result.source_ls_id);
          }
          shards_inner_series[check_result.shard_id]->emplace_back(samples, check_result.ls_id);
        } break;
        default:
          continue;
      }

      stats.samples_added += static_cast<uint32_t>(timeseries_buf_.samples().size());

      if (o.metric_limits == nullptr) {
        continue;
      }

      samples_count += calculate_samples(timeseries_buf_.samples());
      if (o.metric_limits->samples_limit_exceeded(samples_count)) {
        break;
      }
    }

    BareBones::Vector<PromPP::Primitives::Sample> smpl{{def_timestamp, kStaleNan}};
    stale_nan_state.swap(
        [&](uint32_t ls_id) {
          if (auto res = cache.check_input(ls_id); res.status == Cache::CheckResult::kRelabel) {
            shards_inner_series[res.shard_id]->emplace_back(smpl, res.ls_id);
          }
        },
        [&](uint32_t ls_id) {
          if (auto res = cache.check_target(ls_id); res.status == Cache::CheckResult::kKeep) {
            shards_inner_series[shard_id_]->emplace_back(smpl, res.ls_id);
          }
        });
    cache.optimize();
  }

  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE relabelStatus relabel(const RelabelerOptions& o, LabelsBuilder& builder) {
    bool changed = inject_target_labels(builder, o);

    relabelStatus rstatus = stateless_relabeler_->relabeling_process(buf_, builder);
    hard_validate(rstatus, builder, o.metric_limits);
    if (changed && rstatus == rsKeep) {
      rstatus = rsRelabel;
    }

    return rstatus;
  }

  // calculate_samples counts the number of samples excluding stale_nan.
  PROMPP_ALWAYS_INLINE size_t calculate_samples(const BareBones::Vector<PromPP::Primitives::Sample>& samples) noexcept {
    size_t samples_count{0};
    for (const auto smpl : samples) {
      if (is_stale_nan(smpl.value())) {
        continue;
      }
      ++samples_count;
    }

    return samples_count;
  }

 public:
  // inject_target_labels add labels from target to builder.
  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE bool inject_target_labels(LabelsBuilder& target_builder, const RelabelerOptions& o) {
    if (o.target_labels.empty()) {
      return false;
    }

    bool changed{false};

    if (o.honor_labels) {
      for (const auto& [lname, lvalue] : o.target_labels) {
        if (target_builder.contains(static_cast<std::string_view>(lname))) [[unlikely]] {
          continue;
        }
        target_builder.set(static_cast<std::string_view>(lname), static_cast<std::string_view>(lvalue));
        changed = true;
      }
      return changed;
    }

    std::vector<PromPP::Primitives::Label> conflicting_exposed_labels;
    for (const auto& [lname, lvalue] : o.target_labels) {
      PromPP::Primitives::Label existing_label = target_builder.extract(static_cast<std::string_view>(lname));
      if (!existing_label.second.empty()) [[likely]] {
        conflicting_exposed_labels.emplace_back(std::move(existing_label));
      }

      // It is now safe to set the target label.
      target_builder.set(static_cast<std::string_view>(lname), static_cast<std::string_view>(lvalue));
      changed = true;
    }

    // resolve conflict
    if (!conflicting_exposed_labels.empty()) {
      resolve_conflicting_exposed_labels(target_builder, conflicting_exposed_labels);
    }

    return changed;
  }

  // resolve_conflicting_exposed_labels add prefix to conflicting label name.
  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE void resolve_conflicting_exposed_labels(LabelsBuilder& builder, std::vector<PromPP::Primitives::Label>& conflicting_exposed_labels) {
    std::stable_sort(conflicting_exposed_labels.begin(), conflicting_exposed_labels.end(),
                     [](PromPP::Primitives::LabelView a, PromPP::Primitives::LabelView b) { return a.first.size() < b.first.size(); });

    for (auto& [ln, lv] : conflicting_exposed_labels) {
      while (true) {
        ln.insert(0, "exported_");
        if (builder.get(ln).empty()) {
          builder.set(ln, lv);
          break;
        }
      }
    }
  }

  template <class InputLSS, class TargetLSS, hashdex::HashdexInterface Hashdex, class Stats>
  PROMPP_ALWAYS_INLINE void input_relabeling(InputLSS& input_lss,
                                             TargetLSS& target_lss,
                                             Cache& cache,
                                             const Hashdex& hashdex,
                                             const RelabelerOptions& o,
                                             Stats& stats,
                                             PromPP::Primitives::Go::SliceView<InnerSeries*>& shards_inner_series,
                                             PromPP::Primitives::Go::SliceView<RelabeledSeries*>& shards_relabeled_series) {
    NoOpStaleNaNsState state{};
    input_relabeling_internal(input_lss, target_lss, cache, hashdex, o, stats, shards_inner_series, shards_relabeled_series, state,
                              PromPP::Primitives::kNullTimestamp);
  }

  template <class InputLSS, class TargetLSS, hashdex::HashdexInterface Hashdex, class Stats>
  PROMPP_ALWAYS_INLINE void input_relabeling_with_stalenans(InputLSS& input_lss,
                                                            TargetLSS& target_lss,
                                                            Cache& cache,
                                                            const Hashdex& hashdex,
                                                            const RelabelerOptions& o,
                                                            Stats& stats,
                                                            PromPP::Primitives::Go::SliceView<InnerSeries*>& shards_inner_series,
                                                            PromPP::Primitives::Go::SliceView<RelabeledSeries*>& shards_relabeled_series,
                                                            StaleNaNsState& state,
                                                            PromPP::Primitives::Timestamp def_timestamp) {
    input_relabeling_internal(input_lss, target_lss, cache, hashdex, o, stats, shards_inner_series, shards_relabeled_series, state, def_timestamp);
  }

  PROMPP_ALWAYS_INLINE void input_collect_stalenans(Cache& cache,
                                                    PromPP::Primitives::Go::SliceView<InnerSeries*>& shards_inner_series,
                                                    StaleNaNsState& state,
                                                    PromPP::Primitives::Timestamp stale_ts) {
    BareBones::Vector<PromPP::Primitives::Sample> smpl{{stale_ts, kStaleNan}};
    state.swap(
        [&](uint32_t ls_id) {
          if (auto res = cache.check_input(ls_id); res.status == Cache::CheckResult::kRelabel) {
            shards_inner_series[res.shard_id]->emplace_back(smpl, res.ls_id);
          }
        },
        [&](uint32_t ls_id) {
          if (auto res = cache.check_target(ls_id); res.status == Cache::CheckResult::kKeep) {
            shards_inner_series[shard_id_]->emplace_back(smpl, res.ls_id);
          }
        });
    cache.optimize();
  }

  // append_relabeler_series add relabeled ls to lss, add to result and add to cache update(second stage).
  template <class LSS>
  PROMPP_ALWAYS_INLINE void append_relabeler_series(LSS& lss,
                                                    InnerSeries* inner_series,
                                                    const RelabeledSeries* relabeled_series,
                                                    RelabelerStateUpdate* relabeler_state_update) {
    for (const auto& relabeled_serie : relabeled_series->data()) {
      uint32_t ls_id = lss.find_or_emplace(relabeled_serie.ls, relabeled_serie.hash);

      inner_series->emplace_back(relabeled_serie.samples, ls_id);
      relabeler_state_update->emplace_back(relabeled_serie.ls_id, ls_id);
    }
  }

  // update_relabeler_state - add to cache relabled data(third stage).
  PROMPP_ALWAYS_INLINE void update_relabeler_state(Cache& cache, const RelabelerStateUpdate* relabeler_state_update, const uint16_t relabeled_shard_id) {
    for (const auto& update : relabeler_state_update->data()) {
      cache.add_relabel(update.incoming_ls_id, update.relabeled_ls_id, relabeled_shard_id);
    }
  }

  // output_relabeling - relabeling output series(fourth stage).
  template <class LSS>
  PROMPP_ALWAYS_INLINE void output_relabeling(const LSS& lss,
                                              Cache& cache,
                                              RelabeledSeries* relabeled_series,
                                              PromPP::Primitives::Go::SliceView<InnerSeries*>& incoming_inner_series,
                                              PromPP::Primitives::Go::SliceView<InnerSeries*>& encoders_inner_series) {
    std::ranges::for_each(incoming_inner_series, [&](const InnerSeries* inner_series) PROMPP_LAMBDA_INLINE {
      if (inner_series == nullptr || inner_series->size() == 0) {
        return;
      }

      // TODO move ctor builder from ranges for;
      PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state_};

      std::ranges::for_each(inner_series->data(), [&](const InnerSerie& inner_serie) PROMPP_LAMBDA_INLINE {
        auto res = cache.check_input(inner_serie.ls_id);
        if (res.status == Cache::CheckResult::kDrop) {
          return;
        }

        if (res.status == Cache::CheckResult::kRelabel) {
          encoders_inner_series[res.shard_id]->emplace_back(inner_serie.samples, res.ls_id);
          return;
        }

        if (inner_serie.ls_id >= lss.size()) [[unlikely]] {
          throw BareBones::Exception(0x7763a97e1717e835, "ls_id out of range: %d size: %d shard_id: %d", inner_serie.ls_id, lss.size(), shard_id_);
        }
        typename LSS::value_type labels = lss[inner_serie.ls_id];
        builder.reset(labels);
        process_external_labels(builder, external_labels_);

        relabelStatus rstatus = stateless_relabeler_->relabeling_process(buf_, builder);
        soft_validate(rstatus, builder);
        if (rstatus == rsDrop) {
          cache.add_drop(inner_serie.ls_id);
          return;
        }

        PromPP::Primitives::LabelSet new_label_set = builder.label_set();
        relabeled_series->emplace_back(new_label_set, inner_serie.samples, hash_value(new_label_set), inner_serie.ls_id);
      });
    });

    cache.optimize();
  }

  // reset set new number_of_shards and external_labels.
  PROMPP_ALWAYS_INLINE void reset_to(
      const PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>>& external_labels,
      const uint16_t number_of_shards) {
    number_of_shards_ = number_of_shards;
    external_labels_.clear();
    external_labels_.reserve(external_labels.size());
    for (const auto& [ln, lv] : external_labels) {
      external_labels_.emplace_back(static_cast<std::string_view>(ln), static_cast<std::string_view>(lv));
    }
  }

  PROMPP_ALWAYS_INLINE ~PerShardRelabeler() = default;
};

}  // namespace PromPP::Prometheus::Relabel
