#pragma once

#include <vector>

#include <parallel_hashmap/btree.h>
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
};

struct ShardRefSample {
  Primitives::Go::SliceView<RefSample> ref_samples;
  uint16_t shard_id{};
};

using BaseOutputDecoder = BasicDecoder<std::remove_reference_t<Primitives::SnugComposites::LabelSet::ShrinkableEncodingBimap>>;

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

    // write dump type lss and write delta checkpoints
    auto current_cp = output_lss_.checkpoint();
    out << current_cp - dumped_checkpoint_;
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
    BaseOutputDecoder::process_segment(
        [&](Primitives::LabelSetID ls_id, Primitives::Timestamp ts, Primitives::Sample::value_type v) { func(cache_[ls_id], ts, v); });
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
  // ProtobufEncoder constructor over go slice.
  PROMPP_ALWAYS_INLINE explicit GoSliceSink(Primitives::Go::Slice<char>& out) : out_(out) {}

  // Append implementation snappy::Sink.
  PROMPP_ALWAYS_INLINE void Append(const char* data, size_t len) override {
    out_.reserve(len);
    out_.push_back(data, data + len);
  }
};

class ProtobufEncoder {
  std::vector<Primitives::SnugComposites::LabelSet::EncodingBimap*>& output_lsses_;
  phmap::btree_map<std::pair<uint32_t, uint16_t>, BareBones::Vector<Primitives::Sample>> cache_;

 public:
  // ProtobufEncoder constructor.
  PROMPP_ALWAYS_INLINE explicit ProtobufEncoder(std::vector<Primitives::SnugComposites::LabelSet::EncodingBimap*>& output_lsses) noexcept
      : output_lsses_(output_lsses) {}

  // encode incoming refsamples to snapped protobufs on shards.
  PROMPP_ALWAYS_INLINE void encode(Primitives::Go::SliceView<ShardRefSample*>& batch, Primitives::Go::Slice<Primitives::Go::Slice<char>>& out_slices) {
    if (out_slices.empty()) [[unlikely]] {
      // if shards scale 0, do nothing
      return;
    }
    size_t shards = out_slices.size();

    // grouping samples by ls id and main shard id
    for (const auto* srs : batch) {
      for (const auto& rs : srs->ref_samples) {
        cache_[{rs.id, srs->shard_id}].emplace_back(rs.t, rs.v);
      }
    }

    // make containers for output protobufs
    std::vector<std::string> protobufs;
    protobufs.resize(shards);

    // make protobufs from group for output shards
    Primitives::BasicTimeseries<Primitives::SnugComposites::LabelSet::EncodingBimap::value_type*, BareBones::Vector<Primitives::Sample>*> timeseries;
    for (auto& [key, samples] : cache_) {
      auto out_ls = (*output_lsses_[key.second])[key.first];
      timeseries.set_label_set(&out_ls);
      std::sort(samples.begin(), samples.end(),
                [](Primitives::Sample& a, Primitives::Sample& b) PROMPP_LAMBDA_INLINE { return a.timestamp() < b.timestamp(); });
      timeseries.set_samples(&samples);

      protozero::pbf_writer writer(protobufs[static_cast<size_t>(key.first) % shards]);
      Prometheus::RemoteWrite::write_timeseries(writer, timeseries);
      cache_.erase(key);
    }

    // compress to snappy
    for (size_t i = 0; i < out_slices.size(); ++i) {
      if (protobufs[i].empty()) [[unlikely]] {
        continue;
      }

      GoSliceSink writer(out_slices[i]);
      snappy::ByteArraySource reader(protobufs[i].c_str(), protobufs[i].size());
      snappy::Compress(&reader, &writer);
    }
  }
};

}  // namespace PromPP::WAL
