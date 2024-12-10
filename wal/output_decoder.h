#pragma once

#include <vector>

#include "snappy-sinksource.h"
#include "snappy.h"
#define PROTOZERO_USE_VIEW std::string_view
#include "third_party/protozero/pbf_writer.hpp"

#include "bare_bones/preprocess.h"
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

using BaseOutputDecoder = BasicDecoder<Primitives::SnugComposites::LabelSet::ShrinkableEncodingBimap>;

class OutputDecoder : private BaseOutputDecoder {
  Primitives::SnugComposites::LabelSet::ShrinkableEncodingBimap wal_lss_;
  std::vector<uint32_t> cache_{};
  std::stringstream buf_;
  Primitives::LabelsBuilderStateMap builder_state_;
  std::vector<Primitives::LabelView> external_labels_{};
  Prometheus::Relabel::StatelessRelabeler& stateless_relabeler_;
  Primitives::SnugComposites::LabelSet::EncodingBimap& output_lss_;
  Primitives::SnugComposites::LabelSet::EncodingBimap::checkpoint_type dumped_checkpoint_{output_lss_.checkpoint()};
  uint32_t dumped_cache_size_{0};

  // align_cache_to_lss add new labels from lss via relabeler to cache.
  PROMPP_ALWAYS_INLINE void align_cache_to_lss() {
    if (wal_lss_.next_item_index() <= cache_.size()) {
      return;
    }

    Primitives::LabelsBuilder<Primitives::LabelsBuilderStateMap> builder{builder_state_};
    cache_.reserve(wal_lss_.next_item_index());
    for (size_t ls_id = cache_.size(); ls_id < wal_lss_.next_item_index(); ++ls_id) {
      builder.reset(wal_lss_[ls_id]);
      Prometheus::Relabel::process_external_labels(builder, external_labels_);
      Prometheus::Relabel::relabelStatus rstatus = stateless_relabeler_.relabeling_process(buf_, builder);
      Prometheus::Relabel::soft_validate(rstatus, builder);

      if (rstatus == Prometheus::Relabel::rsDrop) {
        cache_.emplace_back(std::numeric_limits<uint32_t>::max());
        continue;
      }

      cache_.emplace_back(output_lss_.find_or_emplace(builder.label_view_set()));
    }

    wal_lss_.shrink_to_checkpoint_size(wal_lss_.checkpoint());
  }

  // load_segment override private load_segment from BaseOutputDecoder.
  template <class InputStream>
  PROMPP_ALWAYS_INLINE void load_segment(InputStream& in, BaseOutputDecoder& wal) {
    in >> wal;
  }

  // dump_cache dump delta cache to output stream.
  template <class OutputStream>
  PROMPP_ALWAYS_INLINE uint32_t dump_cache(uint32_t from, OutputStream& out) {
    // write version
    out.put(1);

    // write size
    uint32_t dumped_ids{0};
    if (from < cache_.size()) [[likely]] {
      dumped_ids = static_cast<uint32_t>(cache_.size()) - from;
    }
    out.write(reinterpret_cast<const char*>(&dumped_ids), sizeof(dumped_ids));

    // if there are no items to write, we finish here
    if (dumped_ids) [[likely]] {
      // write data
      out.write(reinterpret_cast<const char*>(&cache_[from]), sizeof(uint32_t) * dumped_ids);
    }

    return dumped_ids;
  }

  // load_cache load cache from incoming stream.
  template <class InputStream>
  PROMPP_ALWAYS_INLINE void load_cache(InputStream& in) {
    size_t cache_size_before = cache_.size();
    auto sg1 = std::experimental::scope_fail([&]() { cache_.resize(cache_size_before); });

    // read version
    uint8_t version = in.get();

    // return successfully, if stream is empty
    if (in.eof()) {
      return;
    }

    // check version
    if (version != 1) {
      throw BareBones::Exception(0x3f2437abfb3da442, "Invalid cache format version %d while reading from stream, only version 1 is supported", version);
    }

    auto original_exceptions = in.exceptions();
    auto sg2 = std::experimental::scope_exit([&]() { in.exceptions(original_exceptions); });
    in.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit);

    // read size
    uint32_t ids_to_read{0};
    in.read(reinterpret_cast<char*>(&ids_to_read), sizeof(ids_to_read));

    // read is completed, if there are no items
    if (!ids_to_read) {
      return;
    }

    // read data
    size_t last_size{cache_.size()};
    cache_.resize(last_size + static_cast<size_t>(ids_to_read));
    in.read(reinterpret_cast<char*>(&cache_[last_size]), sizeof(uint32_t) * ids_to_read);
  }

 public:
  // WALOutputDecoder constructor with empty state.
  PROMPP_ALWAYS_INLINE explicit OutputDecoder(Prometheus::Relabel::StatelessRelabeler& stateless_relabeler,
                                              Primitives::SnugComposites::LabelSet::EncodingBimap& output_lss,
                                              Primitives::Go::SliceView<std::pair<Primitives::Go::String, Primitives::Go::String>>& external_labels,
                                              BasicEncoderVersion encoder_version = Writer::version) noexcept
      : BaseOutputDecoder{wal_lss_, encoder_version}, stateless_relabeler_{stateless_relabeler}, output_lss_(output_lss) {
    external_labels_.reserve(external_labels.size());
    for (const auto& [ln, lv] : external_labels) {
      external_labels_.emplace_back(static_cast<std::string_view>(ln), static_cast<std::string_view>(lv));
    }
  }

  // cache return current cache.
  PROMPP_ALWAYS_INLINE const auto& cache() const noexcept { return cache_; }

  // dump_to dump delta state(delta caches and delta checkpoint lss) to output stream.
  template <class OutputStream>
  PROMPP_ALWAYS_INLINE void dump_to(OutputStream& out) {
    auto original_exceptions = out.exceptions();
    auto sg1 = std::experimental::scope_exit([&]() { out.exceptions(original_exceptions); });
    out.exceptions(std::ifstream::failbit | std::ifstream::badbit);

    // take current checkpoint and delta with current and previous checkpoints
    auto current_cp = output_lss_.checkpoint();
    auto delta_cp = current_cp - dumped_checkpoint_;

    // no changes - do nothing
    if (delta_cp.empty() && dumped_cache_size_ >= cache_.size()) [[unlikely]] {
      return;
    }

    // write dump type lss and write delta checkpoints
    out << delta_cp;
    dumped_checkpoint_ = std::move(current_cp);

    // write dump type cache and write delta caches
    dumped_cache_size_ += dump_cache(dumped_cache_size_, out);
  }

  // load_from load state(lss and cache) from incoming stream.
  template <class InputStream>
  PROMPP_ALWAYS_INLINE void load_from(InputStream& in) {
    while (true) {
      if (in.eof()) {
        dumped_cache_size_ = cache_.size();
        dumped_checkpoint_ = output_lss_.checkpoint();
        return;
      }

      output_lss_.load(in);
      load_cache(in);
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
    requires std::is_invocable_v<Callback, Primitives::LabelSetID, Primitives::Timestamp, Primitives::Sample::value_type>
  __attribute__((flatten)) void process_segment(Callback&& func) {
    BaseOutputDecoder::process_segment([&](Primitives::LabelSetID ls_id, Primitives::Timestamp ts, Primitives::Sample::value_type v) {
      if (auto id = cache_[ls_id]; id != std::numeric_limits<uint32_t>::max()) {
        func(id, ts, v);
      }
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
  PROMPP_ALWAYS_INLINE void write_compressed_protobuf(Primitives::Go::Slice<char>& compressed) {
    // make protobuf
    protozero::pbf_writer pb_writer(protobuffer_);
    Primitives::SnugComposites::LabelSet::EncodingBimap::value_type last_ls;  // composite_type

    for (size_t lss_shard_id = 0; lss_shard_id < lss_number_of_shards_; ++lss_shard_id) {
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
  PROMPP_ALWAYS_INLINE void encode(Primitives::Go::SliceView<ShardRefSample*>& batch, Primitives::Go::Slice<Primitives::Go::Slice<char>>& out_slices) {
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

      write_compressed_protobuf(out_slices[shard_id]);

      // clear state
      clear_state();
    }

    // shrink state if need
    shrink_if_need();
  }
};

}  // namespace PromPP::WAL
