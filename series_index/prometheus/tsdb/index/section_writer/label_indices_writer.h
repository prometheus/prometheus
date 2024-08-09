#pragma once

#include "bare_bones/preprocess.h"
#include "prometheus/tsdb/index/stream_writer.h"
#include "series_index/prometheus/tsdb/index/types.h"

namespace series_index::prometheus::tsdb::index::section_writer {

template <class Lss>
class LabelIndicesWriter {
 public:
  using StreamWriter = PromPP::Prometheus::tsdb::index::StreamWriter;

  LabelIndicesWriter(const Lss& lss, const SymbolReferencesMap& symbol_references, StreamWriter& writer)
      : lss_(lss), symbol_references_(symbol_references), writer_(writer) {}

  void write_label_indices() {
    uint32_t entries = 0;
    auto entries_placeholder = StreamWriter::write_number_placeholder<uint32_t>(label_indices_table_);

    for (auto name_it = lss_.trie_index().names_trie().make_enumerative_iterator(); name_it.is_valid(); name_it.next()) {
      add_label_indices_table_item(name_it.key());
      generate_label_index(name_it.value(), *lss_.trie_index().values_trie(name_it.value()));
      write_label_index();

      ++entries;
    }

    entries_placeholder.set(entries);
  }

  void write_label_indices_table() {
    writer_.write_uint32(label_indices_table_.size());
    writer_.write(label_indices_table_);
    writer_.compute_and_write_crc32(label_indices_table_);

    free_memory();
  }

 private:
  const Lss& lss_;
  const SymbolReferencesMap& symbol_references_;
  StreamWriter& writer_;

  std::string label_index_;
  std::string label_indices_table_;

  void write_label_index() const {
    writer_.write_uint32(label_index_.size());
    writer_.write(label_index_);
    writer_.compute_and_write_crc32(label_index_);
  }

  void add_label_indices_table_item(std::string_view name) {
    label_indices_table_.push_back(0x01);

    StreamWriter::write_uvarint(name.length(), label_indices_table_);
    label_indices_table_ += name;

    StreamWriter::write_uvarint(writer_.position(), label_indices_table_);
  }

  template <class Trie>
  void generate_label_index(uint32_t name_id, const Trie& values_trie) {
    static constexpr uint32_t kNamesCount = 1;

    label_index_.clear();
    StreamWriter::write_uint32(kNamesCount, label_index_);

    uint32_t entries = 0;
    auto entries_placeholder = StreamWriter::write_number_placeholder<uint32_t>(label_index_);

    for (auto value_it = values_trie.make_enumerative_iterator(); value_it.is_valid(); value_it.next()) {
      StreamWriter::write_uint32(get_symbol_reference(SymbolLssId(name_id, value_it.value())), label_index_);

      ++entries;
    }

    entries_placeholder.set(entries);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE PromPP::Prometheus::tsdb::index::SymbolReference get_symbol_reference(SymbolLssId symbol_id) const noexcept {
    auto reference_it = symbol_references_.find(symbol_id);
    assert(reference_it != symbol_references_.end());
    return reference_it->second;
  }

  PROMPP_ALWAYS_INLINE void free_memory() {
    label_index_.clear();
    label_index_.shrink_to_fit();

    label_indices_table_.clear();
    label_indices_table_.shrink_to_fit();
  }
};

}  // namespace series_index::prometheus::tsdb::index::section_writer