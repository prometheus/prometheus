#pragma once

#include "bare_bones/preprocess.h"
#include "prometheus/tsdb/index/stream_writer.h"
#include "series_index/prometheus/tsdb/index/types.h"

namespace series_index::prometheus::tsdb::index::section_writer {

template <class Lss, class Stream>
class LabelIndicesWriter {
 public:
  using StreamWriter = PromPP::Prometheus::tsdb::index::StreamWriter<Stream>;
  using StringWriter = PromPP::Prometheus::tsdb::index::StringWriter;
  using NoCrc32 = PromPP::Prometheus::tsdb::index::NoCrc32Tag;

  LabelIndicesWriter(const Lss& lss, const SymbolReferencesMap& symbol_references, StreamWriter& writer)
      : lss_(lss), symbol_references_(symbol_references), writer_(writer) {}

  void write_label_indices() {
    indices_table_writer_.write_uint32<NoCrc32>(lss_.reverse_index().names_count());

    for (auto name_it = lss_.trie_index().names_trie().make_enumerative_iterator(); name_it.is_valid(); name_it.next()) {
      add_label_indices_table_item(name_it.key());
      write_label_index(name_it.value(), *lss_.trie_index().values_trie(name_it.value()));
    }
  }

  void write_label_indices_table() {
    const uint32_t payload_size = indices_table_writer_.writer().buffer().size();
    writer_.write_payload(payload_size, [this, payload_size]() mutable {
      writer_.template write_uint32<NoCrc32>(payload_size);
      writer_.write(indices_table_writer_.writer().buffer());
    });

    indices_table_writer_.writer().free_memory();
  }

 private:
  StringWriter indices_table_writer_;

  const Lss& lss_;
  const SymbolReferencesMap& symbol_references_;
  StreamWriter& writer_;

  void add_label_indices_table_item(std::string_view name) {
    indices_table_writer_.write<NoCrc32>(0x01);

    indices_table_writer_.write_varint<NoCrc32>(static_cast<uint64_t>(name.length()));
    indices_table_writer_.write<NoCrc32>(name);

    indices_table_writer_.write_varint<NoCrc32>(static_cast<uint64_t>(writer_.position()));
  }

  template <class Trie>
  void write_label_index(uint32_t name_id, const Trie& values_trie) {
    static constexpr uint32_t kNamesCount = 1;
    const uint32_t values_count = lss_.reverse_index().values_count(name_id);
    const uint32_t payload_size = sizeof(kNamesCount) + sizeof(values_count) + values_count * (sizeof(uint32_t));

    writer_.write_payload(payload_size, [&]() mutable {
      writer_.template write_uint32<NoCrc32>(payload_size);
      writer_.write_uint32(kNamesCount);
      writer_.write_uint32(values_count);

      for (auto value_it = values_trie.make_enumerative_iterator(); value_it.is_valid(); value_it.next()) {
        writer_.write_uint32(get_symbol_reference(SymbolLssId(name_id, value_it.value())));
      }
    });
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE PromPP::Prometheus::tsdb::index::SymbolReference get_symbol_reference(SymbolLssId symbol_id) const noexcept {
    auto reference_it = symbol_references_.find(symbol_id);
    assert(reference_it != symbol_references_.end());
    return reference_it->second;
  }
};

}  // namespace series_index::prometheus::tsdb::index::section_writer