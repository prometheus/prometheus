#pragma once

#include "bare_bones/preprocess.h"
#include "prometheus/tsdb/index/stream_writer.h"
#include "series_index/prometheus/tsdb/index/types.h"

namespace series_index::prometheus::tsdb::index::section_writer {

template <class Lss, class ChunkMetadataList, class Stream>
class SeriesWriter {
 public:
  using StreamWriter = PromPP::Prometheus::tsdb::index::StreamWriter<Stream>;
  using StringWriter = PromPP::Prometheus::tsdb::index::StringWriter;
  using NoCrc32 = PromPP::Prometheus::tsdb::index::NoCrc32Tag;

  static constexpr uint32_t kAllSeries = std::numeric_limits<uint32_t>::max();

  SeriesWriter(const Lss& lss,
               const ChunkMetadataList& chunk_metadata_list,
               const SymbolReferencesMap& symbol_references,
               SeriesReferencesMap& series_references)
      : lss_(lss),
        iterator_(lss_.ls_id_set().begin()),
        end_iterator_(lss_.ls_id_set().end()),
        chunk_metadata_list_(chunk_metadata_list),
        symbol_references_(symbol_references),
        series_references_(series_references) {}

  void write(StreamWriter& writer, uint32_t series_count = kAllSeries) {
    writer.align_to(PromPP::Prometheus::tsdb::index::kSeriesAlignment);

    write_series(writer, series_count);

    if (!has_more_data()) {
      series_writer_.writer().free_memory();
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool has_more_data() const noexcept { return iterator_ != end_iterator_; }

 private:
  const Lss& lss_;
  Lss::LsIdSetIterator iterator_;
  Lss::LsIdSetIterator end_iterator_;
  const ChunkMetadataList& chunk_metadata_list_;
  const SymbolReferencesMap& symbol_references_;
  SeriesReferencesMap& series_references_;

  StringWriter series_writer_;

  void write_series(StreamWriter& writer, uint32_t series_count) {
    for (uint32_t i = 0; has_more_data() && i < series_count; ++i, ++iterator_) {
      auto ls_id = *iterator_;
      emplace_series_reference(ls_id, writer.position());

      series_writer_.writer().clear();

      serialize_labels(ls_id);
      serialize_chunks(ls_id);
      write_serialized_series(writer);
    }
  }

  void serialize_labels(PromPP::Primitives::LabelSetID ls_id) {
    const auto& labels = lss_[ls_id];
    series_writer_.write_varint<NoCrc32>(static_cast<uint64_t>(labels.size()));

    for (auto it = labels.begin(); it != labels.end(); ++it) {
      series_writer_.write_varint<NoCrc32>(static_cast<uint64_t>(get_symbol_reference(SymbolLssId{it.name_id()})));
      series_writer_.write_varint<NoCrc32>(static_cast<uint64_t>(get_symbol_reference(SymbolLssId{it.name_id(), it.value_id()})));
    }
  }

  void serialize_chunks(PromPP::Primitives::LabelSetID ls_id) {
    auto& chunks = chunk_metadata_list_[ls_id];
    series_writer_.write_varint<NoCrc32>(static_cast<uint64_t>(chunks.size()));

    PromPP::Primitives::Timestamp previous_max_timestamp = std::numeric_limits<PromPP::Primitives::Timestamp>::max();
    uint64_t previous_chunk_reference = 0;

    for (auto& chunk : chunks) {
      if (previous_max_timestamp == std::numeric_limits<PromPP::Primitives::Timestamp>::max()) {
        [[unlikely]];
        series_writer_.write_varint<NoCrc32>(static_cast<int64_t>(chunk.min_timestamp));
        series_writer_.write_varint<NoCrc32>(static_cast<uint64_t>(chunk.max_timestamp - chunk.min_timestamp));
        series_writer_.write_varint<NoCrc32>(static_cast<uint64_t>(chunk.reference));
      } else {
        series_writer_.write_varint<NoCrc32>(static_cast<uint64_t>(chunk.min_timestamp - previous_max_timestamp));
        series_writer_.write_varint<NoCrc32>(static_cast<uint64_t>(chunk.max_timestamp - chunk.min_timestamp));
        series_writer_.write_varint<NoCrc32>(static_cast<int64_t>(chunk.reference - previous_chunk_reference));
      }

      previous_chunk_reference = chunk.reference;
      previous_max_timestamp = chunk.max_timestamp;
    }
  }

  PROMPP_ALWAYS_INLINE void emplace_series_reference(PromPP::Primitives::LabelSetID ls_id, size_t position) const {
    auto section_ref = position / PromPP::Prometheus::tsdb::index::kSeriesAlignment;
    series_references_.try_emplace(ls_id, section_ref);
  }

  void write_serialized_series(StreamWriter& writer) const {
    const uint32_t payload_size = series_writer_.writer().buffer().size();
    writer.write_payload(payload_size, [this, &writer]() mutable {
      writer.template write_varint<NoCrc32>(static_cast<uint64_t>(series_writer_.position()));
      writer.write(series_writer_.writer().buffer());
    });

    writer.align_to(PromPP::Prometheus::tsdb::index::kSeriesAlignment);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE PromPP::Prometheus::tsdb::index::SymbolReference get_symbol_reference(SymbolLssId symbol_id) const noexcept {
    auto reference_it = symbol_references_.find(symbol_id);
    assert(reference_it != symbol_references_.end());
    return reference_it->second;
  }
};

}  // namespace series_index::prometheus::tsdb::index::section_writer