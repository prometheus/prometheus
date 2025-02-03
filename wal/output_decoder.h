#pragma once

#include <algorithm>
#include <ranges>
#include <vector>

#include "snappy-sinksource.h"
#include "snappy.h"
#define PROTOZERO_USE_VIEW std::string_view
#include "third_party/protozero/pbf_writer.hpp"

#include "bare_bones/preprocess.h"
#include "bare_bones/serializer.h"
#include "primitives/labels_builder.h"
#include "primitives/snug_composites.h"
#include "prometheus/remote_write.h"
#include "prometheus/stateless_relabeler.h"
#include "wal.h"

namespace PromPP::WAL {

struct RefSample {
  uint32_t id;
  int64_t t;
  double v;

  PROMPP_ALWAYS_INLINE bool operator==(const RefSample&) const noexcept = default;
};

struct ShardRefSample {
  Primitives::Go::SliceView<RefSample> ref_samples;
  uint16_t shard_id{};
};

class OutputDecoderCache {
 public:
  static constexpr auto kIsDropped = Primitives::kInvalidLabelSetID;

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t allocated_memory() const noexcept { return cache_.allocated_memory(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return cache_.size(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool have_changes() const noexcept { return cache_.size() > dumped_cache_size_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Primitives::LabelSetID operator[](Primitives::LabelSetID source_ls_id) const noexcept { return cache_[source_ls_id]; }

  PROMPP_ALWAYS_INLINE void reserve(uint32_t size) noexcept { cache_.reserve(size); }
  PROMPP_ALWAYS_INLINE void add_dropped() noexcept { add(kIsDropped); }
  PROMPP_ALWAYS_INLINE void add(Primitives::LabelSetID ls_id) noexcept { cache_.emplace_back(ls_id); }

  PROMPP_ALWAYS_INLINE void dump_changes(std::ostream& out) {
    BareBones::serialize(out, std::span{&cache_[dumped_cache_size_], cache_.size() - dumped_cache_size_});
    dumped_cache_size_ = cache_.size();
  }

  PROMPP_ALWAYS_INLINE void load_changes(std::istream& in) {
    BareBones::deserialize(in, cache_);
    dumped_cache_size_ = cache_.size();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Primitives::LabelSetID max_ls_id() const noexcept {
    const auto reverse_cache_view = std::ranges::reverse_view(cache_);
    const auto it = std::ranges::find_if(reverse_cache_view, [](Primitives::LabelSetID ls_id) PROMPP_LAMBDA_INLINE { return ls_id != kIsDropped; });
    return it != reverse_cache_view.end() ? *it : kIsDropped;
  }

  bool operator==(const OutputDecoderCache& other) const noexcept { return cache_ == other.cache_; }

 private:
  BareBones::Vector<uint32_t> cache_{};
  uint32_t dumped_cache_size_{};
};

class GorillaSampleDecoderWithSkips {
 public:
  Primitives::Timestamp timestamp_base{std::numeric_limits<Primitives::Timestamp>::max()};

  [[nodiscard]] PROMPP_ALWAYS_INLINE OutputDecoderCache& cache() noexcept { return cache_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const OutputDecoderCache& cache() const noexcept { return cache_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t allocated_memory() const noexcept {
    return cache_.allocated_memory() + gorilla_decoders_.allocated_memory() + null_gorilla_decoders_.allocated_memory();
  }

  PROMPP_ALWAYS_INLINE static void load(std::istream&) {}

  PROMPP_ALWAYS_INLINE static void set_series_count(Primitives::LabelSetID) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE Primitives::Sample decode(Primitives::LabelSetID ls_id,
                                                               Primitives::Timestamp timestamp,
                                                               BareBones::BitSequenceReader& value_sequence,
                                                               SampleCrc&) noexcept {
    return decode_impl(ls_id, timestamp, value_sequence);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Primitives::Sample decode(Primitives::LabelSetID ls_id,
                                                               BareBones::BitSequenceReader& timestamp_sequence,
                                                               BareBones::BitSequenceReader& value_sequence,
                                                               SampleCrc&) noexcept {
    return decode_impl(ls_id, timestamp_sequence, value_sequence);
  }

  PROMPP_ALWAYS_INLINE static SampleCrc::ValidationResult validate_crc(SampleCrc, SampleCrc) noexcept { return SampleCrc::ValidationResult::kValid; }

  PROMPP_ALWAYS_INLINE void sync_decoders_with_cache() {
    null_gorilla_decoders_.resize(cache_.size());

    if (const auto max_ls_id = cache_.max_ls_id(); max_ls_id != OutputDecoderCache::kIsDropped) {
      gorilla_decoders_.resize(max_ls_id + 1);
    }
  }

 private:
  OutputDecoderCache cache_;
  BareBones::Vector<GorillaDecoder> gorilla_decoders_;
  BareBones::Vector<NullGorillaDecoder> null_gorilla_decoders_;

  template <class Timestamp>
  [[nodiscard]] PROMPP_ALWAYS_INLINE Primitives::Sample decode_impl(Primitives::LabelSetID source_ls_id,
                                                                    Timestamp&& timestamp,
                                                                    BareBones::BitSequenceReader& value_sequence) {
    if (source_ls_id >= cache_.size()) [[unlikely]] {
      throw BareBones::Exception(0xf0e57d2a0e5ce7ed, "Error while processing segment LabelSets: Unknown segment's LabelSet's id %d", source_ls_id);
    }

    if (const auto id = cache_[source_ls_id]; id != OutputDecoderCache::kIsDropped) {
      auto& gorilla = gorilla_decoders_[id];
      gorilla.decode(timestamp, value_sequence);
      return {gorilla.last_timestamp() + timestamp_base, gorilla.last_value()};
    }

    null_gorilla_decoders_[source_ls_id].decode(timestamp, value_sequence);
    return {};
  }
};

static_assert(SampleDecoderInterface<GorillaSampleDecoderWithSkips>);

using BaseOutputDecoder = BasicDecoder<Primitives::SnugComposites::LabelSet::ShrinkableEncodingBimap, GorillaSampleDecoderWithSkips>;

class OutputDecoder : private BaseOutputDecoder {
  Primitives::SnugComposites::LabelSet::ShrinkableEncodingBimap wal_lss_;
  std::stringstream buf_;
  Primitives::LabelsBuilderStateMap builder_state_;
  std::vector<Primitives::LabelView> external_labels_{};
  Prometheus::Relabel::StatelessRelabeler& stateless_relabeler_;
  Primitives::SnugComposites::LabelSet::EncodingBimap& output_lss_;
  Primitives::SnugComposites::LabelSet::EncodingBimap::checkpoint_type dumped_checkpoint_{output_lss_.checkpoint()};

  // align_cache_to_lss add new labels from lss via relabeler to cache.
  PROMPP_ALWAYS_INLINE void align_cache_to_lss() {
    auto& cache = sample_decoder().cache();
    if (wal_lss_.next_item_index() <= cache.size()) {
      return;
    }

    Primitives::LabelsBuilder builder{builder_state_};

    cache.reserve(wal_lss_.next_item_index());
    for (size_t ls_id = cache.size(); ls_id < wal_lss_.next_item_index(); ++ls_id) {
      builder.reset(wal_lss_[ls_id]);
      Prometheus::Relabel::process_external_labels(builder, external_labels_);
      Prometheus::Relabel::relabelStatus rstatus = stateless_relabeler_.relabeling_process(buf_, builder);
      Prometheus::Relabel::soft_validate(rstatus, builder);

      if (rstatus == Prometheus::Relabel::rsDrop) {
        cache.add_dropped();
      } else {
        cache.add(output_lss_.find_or_emplace(builder.label_view_set()));
      }
    }

    wal_lss_.shrink_to_checkpoint_size(wal_lss_.checkpoint());
    sample_decoder().sync_decoders_with_cache();
  }

  // load_segment override private load_segment from BaseOutputDecoder.
  template <class InputStream>
  PROMPP_ALWAYS_INLINE void load_segment(InputStream& in, BaseOutputDecoder& wal) {
    in >> wal;
  }

 public:
  // WALOutputDecoder constructor with empty state.
  PROMPP_ALWAYS_INLINE explicit OutputDecoder(Prometheus::Relabel::StatelessRelabeler& stateless_relabeler,
                                              Primitives::SnugComposites::LabelSet::EncodingBimap& output_lss,
                                              Primitives::Go::SliceView<std::pair<Primitives::Go::String, Primitives::Go::String>>& external_labels,
                                              BasicEncoderVersion encoder_version = Writer::version)
      : BaseOutputDecoder{wal_lss_, encoder_version}, stateless_relabeler_{stateless_relabeler}, output_lss_(output_lss) {
    external_labels_.reserve(external_labels.size());
    for (const auto& [ln, lv] : external_labels) {
      external_labels_.emplace_back(static_cast<std::string_view>(ln), static_cast<std::string_view>(lv));
    }
  }

  // cache return current cache.
  PROMPP_ALWAYS_INLINE const auto& cache() const noexcept { return sample_decoder().cache(); }

  // dump_to dump delta state(delta caches and delta checkpoint lss) to output stream.
  PROMPP_ALWAYS_INLINE void dump_to(std::ostream& out) {
    // take current checkpoint and delta with current and previous checkpoints
    auto current_cp = output_lss_.checkpoint();
    const auto delta_cp = current_cp - dumped_checkpoint_;

    // no changes - do nothing
    if (delta_cp.empty() && !cache().have_changes()) [[unlikely]] {
      return;
    }

    // write dump type lss and write delta checkpoints
    out << delta_cp;
    dumped_checkpoint_ = std::move(current_cp);

    // write dump type cache and write delta caches
    sample_decoder().cache().dump_changes(out);
  }

  // load_from load state(lss and cache) from incoming stream.
  template <class InputStream>
  PROMPP_ALWAYS_INLINE void load_from(InputStream& in) {
    while (true) {
      if (in.eof()) {
        dumped_checkpoint_ = output_lss_.checkpoint();
        sample_decoder().sync_decoders_with_cache();
        return;
      }

      output_lss_.load(in);
      sample_decoder().cache().load_changes(in);
    }
  }

  // operator>> override friend operator from BaseOutputDecoder.
  template <class InputStream>
  friend InputStream& operator>>(InputStream& in, OutputDecoder& wal) {
    wal.load_segment(in, wal);
    wal.align_cache_to_lss();
    return in;
  }

  template <class Callback>
    requires std::is_invocable_v<Callback, Primitives::LabelSetID, Primitives::Timestamp, Primitives::Sample::value_type, bool>
  __attribute__((flatten)) void process_segment(Callback&& func) {
    BaseOutputDecoder::process_segment([&](Primitives::LabelSetID ls_id, Primitives::Timestamp ts, Primitives::Sample::value_type v) {
      auto id = sample_decoder().cache()[ls_id];
      func(id, ts, v, id == OutputDecoderCache::kIsDropped);
    });
  }

  template <class Callback>
    requires std::is_invocable_v<Callback, Primitives::LabelSetID, Primitives::Timestamp, Primitives::Sample::value_type>
  __attribute__((flatten)) void process_segment(Callback&& func) {
    process_segment([&](Primitives::LabelSetID ls_id, Primitives::Timestamp ts, Primitives::Sample::value_type v, bool is_dropped) {
      if (is_dropped) {
        return;
      }

      func(ls_id, ts, v);
    });
  }

  template <class Callback>
    requires std::is_invocable_v<Callback, label_set_type, Primitives::Timestamp, Primitives::Sample::value_type>
  __attribute__((flatten)) void process_segment(Callback&& func) {
    process_segment([&](Primitives::LabelSetID ls_id, Primitives::Timestamp ts, Primitives::Sample::value_type v) {
      const auto& label_set = output_lss_[ls_id];

      func(label_set, ts, v);
    });
  }

  template <class Callback>
    requires std::is_invocable_v<Callback, Primitives::LabelSetID, timeseries_type>
  __attribute__((flatten)) void process_segment(Callback&& func) {
    Primitives::BasicTimeseries<Primitives::SnugComposites::LabelSet::ShrinkableEncodingBimap::value_type*> timeseries;
    Primitives::SnugComposites::LabelSet::ShrinkableEncodingBimap::value_type last_ls;  // composite_type
    Primitives::LabelSetID last_ls_id = std::numeric_limits<Primitives::LabelSetID>::max();

    process_segment([&](Primitives::LabelSetID ls_id, Primitives::Timestamp ts, Primitives::Sample::value_type v) {
      if (ls_id != last_ls_id) {
        if (last_ls_id != std::numeric_limits<Primitives::LabelSetID>::max()) {
          func(last_ls_id, timeseries);
        }

        last_ls = output_lss_[ls_id];
        timeseries.set_label_set(&last_ls);
        timeseries.samples().resize(0);
        last_ls_id = ls_id;
      }

      timeseries.samples().push_back(Primitives::Sample(ts, v));
    });

    if (last_ls_id != std::numeric_limits<Primitives::LabelSetID>::max()) {
      func(last_ls_id, timeseries);
    }
  }
};

class GoSliceSink : public snappy::Sink {
  Primitives::Go::Slice<char>& out_;

 public:
  // GoSliceSink constructor over go slice.
  PROMPP_ALWAYS_INLINE explicit GoSliceSink(Primitives::Go::Slice<char>& out) : out_(out) {}

  // Append implementation snappy::Sink.
  PROMPP_ALWAYS_INLINE void Append(const char* data, size_t len) override { out_.push_back(data, data + len); }
};

// ProtobufEncoderStats stats for encoded to snappy protobuf data.
struct ProtobufEncoderStats {
  int64_t max_timestamp{};
  size_t samples_count{};
};

// ProtobufEncoder encoder for snapped protobuf from refSamples.
class ProtobufEncoder {
  Primitives::BasicTimeseries<Primitives::SnugComposites::LabelSet::EncodingBimap::value_type*, BareBones::Vector<Primitives::Sample>> timeseries_;
  std::string protobuffer_;
  std::vector<Primitives::SnugComposites::LabelSet::EncodingBimap*> output_lsses_;
  std::vector<std::vector<const RefSample*>> shards_ref_samples_;
  std::vector<size_t> max_size_shards_ref_samples_;
  size_t lss_number_of_shards_{0};
  size_t max_size_timeseries_samples_{0};
  size_t max_size_protobuffer_{0};

  // write_compressed_protobuf write timeseries to protobuf and compress to snappy.
  PROMPP_ALWAYS_INLINE void write_compressed_protobuf(Primitives::Go::Slice<char>& compressed, WAL::ProtobufEncoderStats& stats) {
    // make protobuf
    protozero::pbf_writer pb_writer(protobuffer_);
    Primitives::SnugComposites::LabelSet::EncodingBimap::value_type last_ls;  // composite_type

    for (size_t lss_shard_id = 0; lss_shard_id < lss_number_of_shards_; ++lss_shard_id) {
      // calculate samples
      stats.samples_count += shards_ref_samples_[lss_shard_id].size();
      // sort by ls id and timestamp
      std::ranges::sort(shards_ref_samples_[lss_shard_id].begin(), shards_ref_samples_[lss_shard_id].end(),
                        [](const RefSample* a, const RefSample* b) PROMPP_LAMBDA_INLINE {
                          if (a->id < b->id) {
                            return true;
                          }

                          if (a->id == b->id) {
                            return a->t < b->t;
                          }

                          return false;
                        });
      uint32_t last_ls_id = PromPP::Primitives::kInvalidLabelSetID;

      for (const RefSample* rsample : shards_ref_samples_[lss_shard_id]) {
        if (rsample->id != last_ls_id) {
          if (last_ls_id != PromPP::Primitives::kInvalidLabelSetID) {
            Prometheus::RemoteWrite::write_timeseries(pb_writer, timeseries_);
          }

          last_ls = (*output_lsses_[lss_shard_id])[rsample->id];
          timeseries_.set_label_set(&last_ls);
          timeseries_.samples().clear();
          last_ls_id = rsample->id;
        }

        timeseries_.samples().emplace_back(rsample->t, rsample->v);

        // max_timestamp in protobuf
        if (stats.max_timestamp < rsample->t) {
          stats.max_timestamp = rsample->t;
        }
      }

      if (last_ls_id != PromPP::Primitives::kInvalidLabelSetID) {
        Prometheus::RemoteWrite::write_timeseries(pb_writer, timeseries_);
      }
    }

    if (protobuffer_.empty()) [[unlikely]] {
      // skip empty protobuf
      return;
    }

    // compress to snappy
    GoSliceSink writer(compressed);
    snappy::ByteArraySource reader(protobuffer_.c_str(), protobuffer_.size());
    snappy::Compress(&reader, &writer);
  }

  // clear_state clear state and maximum size recording.
  PROMPP_ALWAYS_INLINE void clear_state() noexcept {
    max_size_timeseries_samples_ = std::max(max_size_timeseries_samples_, timeseries_.samples().size());
    timeseries_.set_label_set(nullptr);
    timeseries_.samples().clear();

    for (size_t i = 0; i < lss_number_of_shards_; ++i) {
      max_size_shards_ref_samples_[i] = std::max(max_size_shards_ref_samples_[i], shards_ref_samples_[i].size());
      shards_ref_samples_[i].clear();
    }

    max_size_protobuffer_ = std::max(max_size_protobuffer_, protobuffer_.size());
    protobuffer_.clear();
  }

  // shrink_if_need if need shrink state.
  PROMPP_ALWAYS_INLINE void shrink_if_need() noexcept {
    // shrink timeseries samples
    auto& samples = timeseries_.samples();
    if (samples.capacity() != 15 && samples.capacity() > 1.2 * max_size_timeseries_samples_) [[unlikely]] {
      samples.shrink_to_fit();
      samples.reserve(1.1 * max_size_timeseries_samples_);
    }

    // shrink shards_ref_samples
    for (size_t i = 0; i < lss_number_of_shards_; ++i) {
      if (shards_ref_samples_[i].capacity() > 1.2 * max_size_shards_ref_samples_[i]) [[unlikely]] {
        shards_ref_samples_[i].shrink_to_fit();
        shards_ref_samples_[i].reserve(1.1 * max_size_timeseries_samples_);
      }
    }

    // shrink protobuffer
    if (protobuffer_.capacity() > 1 * max_size_protobuffer_) [[unlikely]] {
      protobuffer_.shrink_to_fit();
      protobuffer_.reserve(1.1 * max_size_protobuffer_);
    }
  }

 public:
  // ProtobufEncoder constructor.
  PROMPP_ALWAYS_INLINE explicit ProtobufEncoder(std::vector<Primitives::SnugComposites::LabelSet::EncodingBimap*>&& output_lsses) noexcept
      : output_lsses_{std::move(output_lsses)}, lss_number_of_shards_{output_lsses_.size()} {
    shards_ref_samples_.resize(lss_number_of_shards_);
    max_size_shards_ref_samples_.resize(lss_number_of_shards_);
  }

  // encode incoming refsamples to snapped protobufs on shards.
  // the best algorithm was selected during the tests (comparisons were made with the map cache)
  PROMPP_ALWAYS_INLINE void encode(Primitives::Go::SliceView<ShardRefSample*>& batch,
                                   Primitives::Go::Slice<Primitives::Go::Slice<char>>& out_slices,
                                   Primitives::Go::Slice<WAL::ProtobufEncoderStats>& stats) {
    if (out_slices.empty()) [[unlikely]] {
      // if shards scale 0, do nothing
      return;
    }

    // reset max sizes
    std::ranges::for_each(max_size_shards_ref_samples_, [](auto& max_size) { max_size = 0; });
    max_size_timeseries_samples_ = 0;
    max_size_protobuffer_ = 0;

    // make protobuf from group for output shards
    size_t number_of_shards = out_slices.size();
    for (size_t shard_id = 0; shard_id < number_of_shards; ++shard_id) {
      for (const auto* srs : batch) {
        for (const auto& rs : srs->ref_samples) {
          if ((static_cast<size_t>(rs.id) % number_of_shards) != shard_id) {
            // skip data not for this shard
            continue;
          }

          shards_ref_samples_[srs->shard_id].push_back(&rs);
        }
      }

      write_compressed_protobuf(out_slices[shard_id], stats[shard_id]);

      // clear state
      clear_state();
    }

    // shrink state if need
    shrink_if_need();
  }
};

}  // namespace PromPP::WAL
