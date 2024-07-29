#pragma once

#include "bare_bones/preprocess.h"
#include "prometheus/tsdb/index/stream_writer.h"
#include "series_index/prometheus/tsdb/index/types.h"
#include "series_index/reverse_index.h"

namespace series_index::prometheus::tsdb::index::section_writer {

template <class Lss>
class PostingsWriter {
 public:
  using StreamWriter = PromPP::Prometheus::tsdb::index::StreamWriter;

  PostingsWriter(const Lss& lss, const SeriesReferencesMap& series_references, StreamWriter& writer)
      : lss_(lss), series_references_(series_references), writer_(writer) {}

  void write_postings() {
    auto entries_placeholder = StreamWriter::write_number_placeholder<uint32_t>(postings_table_offsets_);

    write_posting_with_all_series();
    uint32_t entries = 1;

    for (auto name_it = lss_.trie_index().names_trie().make_enumerative_iterator(); name_it.is_valid(); name_it.next()) {
      for (auto value_it = lss_.trie_index().values_trie(name_it.value())->make_enumerative_iterator(); value_it.is_valid(); value_it.next()) {
        add_posting_table_offset_item(name_it.key(), value_it.key());

        const auto* series_id_sequence = lss_.reverse_index().get(name_it.value(), value_it.value());
        assert(series_id_sequence != nullptr);
        generate_series_references(*series_id_sequence);
        generate_posting();
        write_posting();

        ++entries;
      }
    }

    entries_placeholder.set(entries);
  }

  void write_postings_table_offsets() {
    writer_.write_uint32(postings_table_offsets_.size());
    writer_.write(postings_table_offsets_);
    writer_.compute_and_write_crc32(postings_table_offsets_);
  }

 private:
  const Lss& lss_;
  const SeriesReferencesMap& series_references_;
  StreamWriter& writer_;

  std::string posting_;
  std::string postings_table_offsets_;
  std::vector<uint32_t> series_reference_list_;

  void write_posting_with_all_series() {
    add_posting_table_offset_item("", "");

    generate_series_references(series_references_);
    generate_posting();
    series_reference_list_.clear();
    series_reference_list_.shrink_to_fit();

    write_posting();
    posting_.clear();
    posting_.shrink_to_fit();
  }

  void write_posting() {
    writer_.write_uint32(posting_.size());
    writer_.write(posting_);
    writer_.compute_and_write_crc32(posting_);
  }

  void add_posting_table_offset_item(std::string_view name, std::string_view value) {
    postings_table_offsets_.push_back(0x02);

    StreamWriter::write_uvarint(name.length(), postings_table_offsets_);
    postings_table_offsets_ += name;

    StreamWriter::write_uvarint(value.length(), postings_table_offsets_);
    postings_table_offsets_ += value;

    StreamWriter::write_uvarint(writer_.position(), postings_table_offsets_);
  }

  template <class SeriesIdList>
  void generate_series_references(const SeriesIdList& series_id_sequence) {
    const auto get_series_reference = [&](const uint32_t series_id) PROMPP_LAMBDA_INLINE {
      auto it = series_references_.find(series_id);
      assert(it != series_references_.end());
      return it->second;
    };

    series_reference_list_.clear();

    if constexpr (std::is_same_v<SeriesIdList, series_index::CompactSeriesIdSequence>) {
      series_reference_list_.reserve(series_id_sequence.count());
      series_id_sequence.process_series([this, &get_series_reference](const auto& series) PROMPP_LAMBDA_INLINE {
        std::ranges::transform(series, std::back_inserter(series_reference_list_), get_series_reference);
      });
    } else {
      series_reference_list_.reserve(series_id_sequence.size());
      std::ranges::transform(series_id_sequence, std::back_inserter(series_reference_list_),
                             [](const auto& iterator) PROMPP_LAMBDA_INLINE { return iterator.second; });
    }

    std::ranges::sort(series_reference_list_);
  }

  void generate_posting() {
    const auto size = sizeof(uint32_t) + series_reference_list_.size() * sizeof(PromPP::Prometheus::tsdb::index::SeriesReference);

    posting_.clear();
    posting_.reserve(size);
    StreamWriter::write_uint32(series_reference_list_.size(), posting_);

    for (auto series_reference : series_reference_list_) {
      StreamWriter::write_uint32(series_reference, posting_);
    }
  }
};

}  // namespace series_index::prometheus::tsdb::index::section_writer