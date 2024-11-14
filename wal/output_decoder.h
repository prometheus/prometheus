#pragma once

#include <vector>

#include "bare_bones/preprocess.h"
#include "primitives/snug_composites.h"
#include "prometheus/stateless_relabeler.h"
#include "wal.h"

namespace PromPP::WAL {

enum dumpType : uint8_t {
  // dUnknownType - unknown type.
  dUnknownType = 0,
  // dLSS lss type.
  dLSS,
  // dCache cache type.
  dCache,
};

using BaseOutputDecoder = WAL::BasicDecoder<std::remove_reference_t<Primitives::SnugComposites::LabelSet::EncodingTable>>;

class WALOutputDecoder : public BaseOutputDecoder {
  Primitives::SnugComposites::LabelSet::EncodingTable wal_lss_;
  std::vector<uint32_t> cache_{};
  std::stringstream buf_;
  Primitives::LabelsBuilderStateMap builder_state_;
  std::vector<Primitives::LabelView> external_labels_{};
  Prometheus::Relabel::StatelessRelabeler& stateless_relabeler_;
  Primitives::SnugComposites::LabelSet::EncodingBimap& output_lss_;
  Primitives::SnugComposites::LabelSet::EncodingBimap::checkpoint_type dumped_checkpoint_{output_lss_.checkpoint()};
  uint32_t dumped_cache_size_{0};

 public:
  // WALOutputDecoder constructor with empty state.
  PROMPP_ALWAYS_INLINE explicit WALOutputDecoder(Prometheus::Relabel::StatelessRelabeler& stateless_relabeler,
                                                 Primitives::SnugComposites::LabelSet::EncodingBimap& output_lss,
                                                 Primitives::Go::SliceView<std::pair<Primitives::Go::String, Primitives::Go::String>>& external_labels,
                                                 BasicEncoderVersion encoder_version = WAL::Writer::version) noexcept
      : BaseOutputDecoder{wal_lss_, encoder_version}, stateless_relabeler_{stateless_relabeler}, output_lss_(output_lss) {
    external_labels_.reserve(external_labels.size());
    for (const auto& [ln, lv] : external_labels) {
      external_labels_.emplace_back(static_cast<std::string_view>(ln), static_cast<std::string_view>(lv));
    }
  }

  // dump_cache dump delta cache to output stream.
  template <class OutputStream>
  PROMPP_ALWAYS_INLINE uint32_t dump_cache(uint32_t from, OutputStream& out) {
    auto original_exceptions = out.exceptions();
    auto sg1 = std::experimental::scope_exit([&]() { out.exceptions(original_exceptions); });
    out.exceptions(std::ifstream::failbit | std::ifstream::badbit);

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

  // dump_to dump delta state(delta caches and delta checkpoint lss) to output stream.
  template <class OutputStream>
  PROMPP_ALWAYS_INLINE void dump_to(OutputStream& out) {
    auto original_exceptions = out.exceptions();
    auto sg1 = std::experimental::scope_exit([&]() { out.exceptions(original_exceptions); });
    out.exceptions(std::ifstream::failbit | std::ifstream::badbit);

    // write dump type lss and write delta checkpoints
    out.put(dLSS);
    auto current_cp = output_lss_.checkpoint();
    out << current_cp - dumped_checkpoint_;
    dumped_checkpoint_ = std::move(current_cp);

    // write dump type cache and write delta caches
    out.put(dCache);
    dumped_cache_size_ += dump_cache(dumped_cache_size_, out);
  }

  // load_cache load cache from incoming stream.
  template <class InputStream>
  PROMPP_ALWAYS_INLINE void load_cache(InputStream& in) {
    auto sg1 = std::experimental::scope_fail([&]() { cache_.clear(); });

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

  // load_from load state(lss and cache) from incoming stream.
  template <class InputStream>
  PROMPP_ALWAYS_INLINE void load_from(InputStream& in) {
    while (true) {
      // read dump type
      uint8_t dump_type = in.get();

      if (in.eof()) {
        dumped_cache_size_ = cache_.size();
        dumped_checkpoint_ = output_lss_.checkpoint();
        return;
      }

      // switch on dump type load
      switch (dump_type) {
        case dLSS: {
          output_lss_.load(in);
          break;
        }

        case dCache: {
          load_cache(in);
          break;
        }

        default: {
          throw BareBones::Exception(0x3f2437abfb3da442, "invalid restore dump type %d while reading from stream", dump_type);
        }
      }
    }
  }

  PROMPP_ALWAYS_INLINE void align_cache_to_lss() {
    if (wal_lss_.size() <= cache_.size()) {
      return;
    }

    Primitives::LabelsBuilder<Primitives::LabelsBuilderStateMap> builder{builder_state_};
    for (size_t ls_id = cache_.size(); ls_id < wal_lss_.size(); ++ls_id) {
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

    // wal_lss_.shrink_to_checkpoint_size(wal_lss_.checkpoint());
  }

  template <class Callback>
    requires std::is_invocable_v<Callback, Primitives::LabelSetID, Primitives::Timestamp, Primitives::Sample::value_type>
  __attribute__((flatten)) void process_segment(Callback&& func) {
    align_cache_to_lss();

    BaseOutputDecoder::process_segment(
        [&](Primitives::LabelSetID ls_id, Primitives::Timestamp ts, Primitives::Sample::value_type v) { func(cache_[ls_id], ts, v); });
  }

  // cache return current cache.
  PROMPP_ALWAYS_INLINE const auto& cache() const noexcept { return cache_; }
};

}  // namespace PromPP::WAL
