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

  static constexpr uint32_t kUnlimitedBatchSize = std::numeric_limits<uint32_t>::max();

  PostingsWriter(const Lss& lss, const SeriesReferencesMap& series_references, StreamWriter& writer)
      : lss_(lss), trie_index_iterator_(lss_.trie_index().begin()), series_references_(series_references), writer_(writer) {}

  void write_postings(uint32_t max_batch_size = kUnlimitedBatchSize) {
    auto const is_batch_filled = [this, max_batch_size, writer_start_position = writer_.position()]()
                                     PROMPP_LAMBDA_INLINE { return writer_.position() - writer_start_position >= max_batch_size; };

    if (!entries_placeholder_) {
      entries_placeholder_ = StreamWriter::write_number_placeholder<uint32_t>(postings_table_offsets_);
      write_posting_with_all_series();

      if (is_batch_filled()) {
        return;
      }
    }

    while (has_more_data()) {
      auto& item = *trie_index_iterator_;
      write_posting(get_series_ids_sequence(item.name_id(), item.value_id()), item.name(), item.value());
      ++trie_index_iterator_;

      if (is_batch_filled()) {
        return;
      }
    }
  }

  void write_postings_table_offsets() {
    entries_placeholder_->set(entries_);

    writer_.write_uint32(postings_table_offsets_.size());
    writer_.write(postings_table_offsets_);
    writer_.compute_and_write_crc32(postings_table_offsets_);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool has_more_data() const noexcept { return trie_index_iterator_ != lss_.trie_index().end(); }

 private:
  const Lss& lss_;
  Lss::TrieIndexIterator trie_index_iterator_;
  const SeriesReferencesMap& series_references_;
  StreamWriter& writer_;

  std::optional<StreamWriter::NumberPlaceholder<uint32_t>> entries_placeholder_;
  std::string serialized_posting_;
  std::string postings_table_offsets_;
  std::vector<uint32_t> series_reference_list_;
  uint32_t entries_{};

  void write_posting_with_all_series() {
    write_posting(series_references_, "", "");

    series_reference_list_.clear();
    series_reference_list_.shrink_to_fit();

    serialized_posting_.clear();
    serialized_posting_.shrink_to_fit();
  }

  template <class SeriesReference>
  void write_posting(const SeriesReference& series_reference, std::string_view name, std::string_view value) {
    add_posting_table_offset_item(name, value, writer_.position());

    generate_series_references(series_reference);
    serialize_posting();
    write_serialized_posting();

    ++entries_;
  }

  void write_serialized_posting() {
    writer_.write_uint32(serialized_posting_.size());
    writer_.write(serialized_posting_);
    writer_.compute_and_write_crc32(serialized_posting_);
  }

  void add_posting_table_offset_item(std::string_view name, std::string_view value, size_t position) {
    postings_table_offsets_.push_back(0x02);

    StreamWriter::write_uvarint(name.length(), postings_table_offsets_);
    postings_table_offsets_ += name;

    StreamWriter::write_uvarint(value.length(), postings_table_offsets_);
    postings_table_offsets_ += value;

    StreamWriter::write_uvarint(position, postings_table_offsets_);
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

  void serialize_posting() {
    const auto size = sizeof(uint32_t) + series_reference_list_.size() * sizeof(PromPP::Prometheus::tsdb::index::SeriesReference);

    serialized_posting_.clear();
    serialized_posting_.reserve(size);
    StreamWriter::write_uint32(series_reference_list_.size(), serialized_posting_);

    for (auto series_reference : series_reference_list_) {
      StreamWriter::write_uint32(series_reference, serialized_posting_);
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const CompactSeriesIdSequence& get_series_ids_sequence(uint32_t name_id, uint32_t value_id) const noexcept {
    const auto* series_id_sequence = lss_.reverse_index().get(name_id, value_id);
    assert(series_id_sequence != nullptr);
    return *series_id_sequence;
  }
};

}  // namespace series_index::prometheus::tsdb::index::section_writer