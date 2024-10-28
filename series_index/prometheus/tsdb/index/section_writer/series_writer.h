#pragma once

#include "bare_bones/preprocess.h"
#include "prometheus/tsdb/index/stream_writer.h"
#include "series_index/prometheus/tsdb/index/types.h"

namespace series_index::prometheus::tsdb::index::section_writer {

template <class Lss, class Stream>
class SeriesWriter {
 public:
  using StreamWriter = PromPP::Prometheus::tsdb::index::StreamWriter<Stream>;
  using StringWriter = PromPP::Prometheus::tsdb::index::StringWriter;
  using NoCrc32 = PromPP::Prometheus::tsdb::index::NoCrc32Tag;

  SeriesWriter(const Lss& lss, const SymbolReferencesMap& symbol_references, SeriesReferencesMap& series_references)
      : lss_(lss), symbol_references_(symbol_references), series_references_(series_references) {}

  template <class ChunkMetadataContainer>
  void write(PromPP::Primitives::LabelSetID ls_id, const ChunkMetadataContainer& chunks, StreamWriter& writer) {
    writer.align_to(PromPP::Prometheus::tsdb::index::kSeriesAlignment);

    emplace_series_reference(ls_id, writer.position());

    series_writer_.writer().clear();

    serialize_labels(ls_id);
    serialize_chunks(chunks);
    write_serialized_series(writer);
  }

 private:
  const Lss& lss_;
  const SymbolReferencesMap& symbol_references_;
  SeriesReferencesMap& series_references_;

  StringWriter series_writer_;

  void serialize_labels(PromPP::Primitives::LabelSetID ls_id) {
    const auto& labels = lss_[ls_id];
    series_writer_.write_varint<NoCrc32>(static_cast<uint64_t>(labels.size()));

    for (auto it = labels.begin(); it != labels.end(); ++it) {
      series_writer_.write_varint<NoCrc32>(static_cast<uint64_t>(get_symbol_reference(SymbolLssId{it.name_id()})));
      series_writer_.write_varint<NoCrc32>(static_cast<uint64_t>(get_symbol_reference(SymbolLssId{it.name_id(), it.value_id()})));
    }
  }

  template <class ChunkMetadataContainer>
  void serialize_chunks(const ChunkMetadataContainer& chunks) {
    series_writer_.write_varint<NoCrc32>(static_cast<uint64_t>(chunks.size()));

    auto previous_max_timestamp = std::numeric_limits<PromPP::Primitives::Timestamp>::max();
    uint64_t previous_chunk_reference = 0;

    for (auto& chunk : chunks) {
      if (previous_max_timestamp == std::numeric_limits<PromPP::Primitives::Timestamp>::max()) [[unlikely]] {
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
    const auto reference_it = symbol_references_.find(symbol_id);
    assert(reference_it != symbol_references_.end());
    return reference_it->second;
  }
};

}  // namespace series_index::prometheus::tsdb::index::section_writer