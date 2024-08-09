#pragma once

#include "bare_bones/preprocess.h"
#include "prometheus/tsdb/index/stream_writer.h"
#include "series_index/prometheus/tsdb/index/types.h"

namespace series_index::prometheus::tsdb::index::section_writer {

template <class Lss, class ChunkMetadataList>
class SeriesWriter {
 public:
  using StreamWriter = PromPP::Prometheus::tsdb::index::StreamWriter;

  static constexpr uint32_t kAllSeries = std::numeric_limits<uint32_t>::max();

  SeriesWriter(const Lss& lss,
               const ChunkMetadataList& chunk_metadata_list,
               const SymbolReferencesMap& symbol_references,
               SeriesReferencesMap& series_references)
      : lss_(lss),
        iterator_(lss_.ls_id_set().begin()),
        chunk_metadata_list_(chunk_metadata_list),
        symbol_references_(symbol_references),
        series_references_(series_references) {}

  void write(StreamWriter& writer, uint32_t series_count = kAllSeries) {
    writer.align_to(PromPP::Prometheus::tsdb::index::kSeriesAlignment);

    write_series(writer, series_count);

    if (!has_more_data()) {
      free_memory();
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool has_more_data() const noexcept { return iterator_ != lss_.ls_id_set().end(); }

 private:
  const Lss& lss_;
  Lss::LsIdSetIterator iterator_;
  const ChunkMetadataList& chunk_metadata_list_;
  const SymbolReferencesMap& symbol_references_;
  SeriesReferencesMap& series_references_;

  std::string serialized_series_str_;
  uint64_t chunk_offset_{};

  void write_series(StreamWriter& writer, uint32_t series_count) {
    for (uint32_t i = 0; has_more_data() && i < series_count; ++i, ++iterator_) {
      auto ls_id = *iterator_;
      emplace_series_reference(ls_id, writer.position());

      serialized_series_str_.clear();
      serialize_labels(ls_id);
      serialize_chunks(ls_id);
      write_serialized_series(writer);
    }
  }

  void serialize_labels(PromPP::Primitives::LabelSetID ls_id) {
    const auto& labels = lss_[ls_id];
    StreamWriter::write_uvarint(labels.size(), serialized_series_str_);

    for (auto it = labels.begin(); it != labels.end(); ++it) {
      StreamWriter::write_uvarint(get_symbol_reference(SymbolLssId{it.name_id()}), serialized_series_str_);
      StreamWriter::write_uvarint(get_symbol_reference(SymbolLssId{it.name_id(), it.value_id()}), serialized_series_str_);
    }
  }

  void serialize_chunks(PromPP::Primitives::LabelSetID ls_id) {
    auto& chunks = chunk_metadata_list_[ls_id];
    StreamWriter::write_uvarint(chunks.size(), serialized_series_str_);

    PromPP::Primitives::Timestamp previous_max_timestamp = std::numeric_limits<PromPP::Primitives::Timestamp>::max();
    uint64_t previous_chunk_size = 0;

    for (auto& chunk : chunks) {
      if (previous_max_timestamp == std::numeric_limits<PromPP::Primitives::Timestamp>::max()) {
        StreamWriter::write_varint(chunk.min_timestamp, serialized_series_str_);
        StreamWriter::write_uvarint(chunk.max_timestamp - chunk.min_timestamp, serialized_series_str_);
        StreamWriter::write_uvarint(chunk_offset_, serialized_series_str_);
      } else {
        StreamWriter::write_uvarint(chunk.min_timestamp - previous_max_timestamp, serialized_series_str_);
        StreamWriter::write_uvarint(chunk.max_timestamp - chunk.min_timestamp, serialized_series_str_);
        StreamWriter::write_varint(previous_chunk_size, serialized_series_str_);
      }

      chunk_offset_ += chunk.size;
      previous_chunk_size = chunk.size;
      previous_max_timestamp = chunk.max_timestamp;
    }
  }

  PROMPP_ALWAYS_INLINE void emplace_series_reference(PromPP::Primitives::LabelSetID ls_id, size_t position) const {
    auto section_ref = position / PromPP::Prometheus::tsdb::index::kSeriesAlignment;
    series_references_.try_emplace(ls_id, section_ref);
  }

  void write_serialized_series(StreamWriter& writer) const {
    writer.write_uvarint(serialized_series_str_.length());
    writer.write(serialized_series_str_);
    writer.compute_and_write_crc32(serialized_series_str_);
    writer.align_to(PromPP::Prometheus::tsdb::index::kSeriesAlignment);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE PromPP::Prometheus::tsdb::index::SymbolReference get_symbol_reference(SymbolLssId symbol_id) const noexcept {
    auto reference_it = symbol_references_.find(symbol_id);
    assert(reference_it != symbol_references_.end());
    return reference_it->second;
  }

  PROMPP_ALWAYS_INLINE void free_memory() noexcept {
    serialized_series_str_.clear();
    serialized_series_str_.shrink_to_fit();
  }
};

}  // namespace series_index::prometheus::tsdb::index::section_writer