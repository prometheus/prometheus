#pragma once

#include "bare_bones/preprocess.h"
#include "prometheus/tsdb/index/stream_writer.h"
#include "series_index/prometheus/tsdb/index/types.h"

namespace series_index::prometheus::tsdb::index::section_writer {

template <class Lss>
class SeriesWriter {
 public:
  using StreamWriter = PromPP::Prometheus::tsdb::index::StreamWriter;

  SeriesWriter(const Lss& lss, const SymbolReferencesMap& symbol_references, SeriesReferencesMap& series_references, StreamWriter& writer)
      : lss_(lss), symbol_references_(symbol_references), series_references_(series_references), writer_(writer) {}

  void write() {
    writer_.align_to(PromPP::Prometheus::tsdb::index::kSeriesAlignment);

    for (auto ls_id : lss_.ls_id_set()) {
      emplace_series_reference(ls_id);

      serialized_series_str_.clear();
      serialize_labels(ls_id);
      serialize_chunks();
      write_series();
    }
  }

 private:
  const Lss& lss_;
  const SymbolReferencesMap& symbol_references_;
  SeriesReferencesMap& series_references_;
  StreamWriter& writer_;

  std::string serialized_series_str_;

  void serialize_labels(PromPP::Primitives::LabelSetID ls_id) {
    const auto& labels = lss_[ls_id];
    StreamWriter::write_uvarint(labels.size(), serialized_series_str_);

    for (auto it = labels.begin(); it != labels.end(); ++it) {
      StreamWriter::write_uvarint(get_symbol_reference(SymbolLssId{it.name_id()}), serialized_series_str_);
      StreamWriter::write_uvarint(get_symbol_reference(SymbolLssId{it.name_id(), it.value_id()}), serialized_series_str_);
    }
  }

  void serialize_chunks() {
    StreamWriter::write_uvarint(0, serialized_series_str_);

#if 0
    // chunk1
    StreamWriter::write_varint(1720184415258, serialized_series_str_);
    StreamWriter::write_uvarint(7200000, serialized_series_str_);
    StreamWriter::write_uvarint(0, serialized_series_str_);

    // chunk2
    StreamWriter::write_uvarint(1000, serialized_series_str_);
    StreamWriter::write_uvarint(7200000, serialized_series_str_);
    StreamWriter::write_uvarint(0, serialized_series_str_);
#endif
  }

  PROMPP_ALWAYS_INLINE void emplace_series_reference(PromPP::Primitives::LabelSetID ls_id) {
    auto section_ref = writer_.position() / PromPP::Prometheus::tsdb::index::kSeriesAlignment;
    series_references_.try_emplace(ls_id, section_ref);
  }

  void write_series() {
    writer_.write_uvarint(serialized_series_str_.length());
    writer_.write(serialized_series_str_);
    writer_.compute_and_write_crc32(serialized_series_str_);
    writer_.align_to(PromPP::Prometheus::tsdb::index::kSeriesAlignment);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE PromPP::Prometheus::tsdb::index::SymbolReference get_symbol_reference(SymbolLssId symbol_id) const noexcept {
    auto reference_it = symbol_references_.find(symbol_id);
    assert(reference_it != symbol_references_.end());
    return reference_it->second;
  }
};

}  // namespace series_index::prometheus::tsdb::index::section_writer