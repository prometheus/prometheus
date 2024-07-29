#pragma once

#include "bare_bones/preprocess.h"
#include "prometheus/tsdb/index/stream_writer.h"
#include "series_index/prometheus/tsdb/index/types.h"

namespace series_index::prometheus::tsdb::index::section_writer {

template <class Lss>
class SymbolsWriter {
 public:
  using StreamWriter = PromPP::Prometheus::tsdb::index::StreamWriter;

  SymbolsWriter(const Lss& lss, SymbolReferencesMap& symbol_references, StreamWriter& writer)
      : lss_(lss), symbol_references_(symbol_references), writer_(writer) {}

  void write() {
    generate_symbol_id_list();
    generate_symbol_references();
    write_symbols();
  }

 private:
  static constexpr SymbolLssId kEmptySymbol{};

  const Lss& lss_;
  SymbolReferencesMap& symbol_references_;
  StreamWriter& writer_;

  std::vector<SymbolLssId> symbol_ids_;
  uint32_t serialized_symbols_length_ = 0;

  void generate_symbol_id_list() {
    symbol_ids_.reserve(get_symbols_count() + 1);
    symbol_ids_.emplace_back(kEmptySymbol);

    auto& names = lss_.data().label_name_sets_table.data().symbols_table;
    for (uint32_t name_id = 0; name_id < names.size(); ++name_id) {
      symbol_ids_.emplace_back(name_id);

      serialized_symbols_length_ += serialized_string_length(names[name_id]);

      auto& values = *lss_.data().symbols_tables[name_id];
      for (uint32_t value_id = 0; value_id < values.size(); ++value_id) {
        symbol_ids_.emplace_back(name_id, value_id);

        serialized_symbols_length_ += serialized_string_length(values[value_id]);
      }
    }

    std::ranges::sort(symbol_ids_, [this](SymbolLssId a, SymbolLssId b) PROMPP_LAMBDA_INLINE { return get_symbol(a) < get_symbol(b); });
    auto unique_it = std::ranges::unique(symbol_ids_, [this](SymbolLssId a, SymbolLssId b) PROMPP_LAMBDA_INLINE { return get_symbol(a) == get_symbol(b); });
    symbol_ids_.erase(unique_it.begin(), unique_it.end());
  }

  [[nodiscard]] uint32_t get_symbols_count() const noexcept {
    auto& names = lss_.data().label_name_sets_table.data().symbols_table;
    uint32_t count = names.size();
    for (uint32_t name_id = 0; name_id < names.size(); ++name_id) {
      count += lss_.data().symbols_tables[name_id]->size();
    }

    return count;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static uint32_t serialized_string_length(const std::string_view& str) noexcept {
    return BareBones::Encoding::VarInt::kMaxVarIntLength + str.length();
  }

  void generate_symbol_references() {
    const auto symbol_and_symbol_id_comparator = [this](SymbolLssId a, const std::string_view& b) PROMPP_LAMBDA_INLINE { return get_symbol(a) < b; };
    const auto emplace_symbol_reference = [this, &symbol_and_symbol_id_comparator](std::string_view symbol, SymbolLssId symbol_id) PROMPP_LAMBDA_INLINE {
      auto it = std::lower_bound(symbol_ids_.begin(), symbol_ids_.end(), symbol, symbol_and_symbol_id_comparator);
      symbol_references_.try_emplace(symbol_id, std::distance(symbol_ids_.begin(), it));
    };

    auto& names = lss_.data().label_name_sets_table.data().symbols_table;
    for (uint32_t name_id = 0; name_id < names.size(); ++name_id) {
      emplace_symbol_reference(names[name_id], SymbolLssId(name_id));

      auto& values = *lss_.data().symbols_tables[name_id];
      for (uint32_t value_id = 0; value_id < values.size(); ++value_id) {
        emplace_symbol_reference(values[value_id], SymbolLssId(name_id, value_id));
      }
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view get_symbol(SymbolLssId symbol_id) const noexcept {
    if (symbol_id.is_empty()) {
      [[unlikely]];
      return "";
    } else if (symbol_id.is_name()) {
      return lss_.data().label_name_sets_table.data().symbols_table[symbol_id.name_id];
    }

    return lss_.data().symbols_tables[symbol_id.name_id]->operator[](symbol_id.value_id);
  }

  void write_symbols() noexcept {
    std::string symbols_str;
    symbols_str.reserve(sizeof(uint32_t) + serialized_symbols_length_);
    StreamWriter::write_uint32(symbol_ids_.size(), symbols_str);

    for (auto symbol_id : symbol_ids_) {
      auto symbol = get_symbol(symbol_id);
      StreamWriter::write_uvarint(symbol.length(), symbols_str);
      symbols_str += symbol;
    }

    writer_.write_uint32(symbols_str.size());
    writer_.write(symbols_str);
    writer_.compute_and_write_crc32(symbols_str);
  }
};

}  // namespace series_index::prometheus::tsdb::index::section_writer