#pragma once

#include <string_view>

namespace PromPP::Primitives::lss_metadata {

struct PROMPP_ATTRIBUTE_PACKED SymbolsTableCheckpoint {
  uint32_t symbols_count{};
  uint32_t symbols_data_size{};
};

struct Metadata {
  std::string_view help;
  std::string_view type;
  std::string_view unit;

  auto operator<=>(const Metadata&) const noexcept = default;
};

template <class ChangesCollector>
concept ChangesCollectorInterface = requires(ChangesCollector& collector, const ChangesCollector& const_collector, const SymbolsTableCheckpoint& checkpoint) {
  { const_collector.count() } -> std::same_as<uint32_t>;
  { const_collector.symbols_table_checkpoint() } -> std::same_as<const SymbolsTableCheckpoint&>;

  { const_collector.begin() };
  { const_collector.end() };

  { const_collector.allocated_memory() } -> std::same_as<uint32_t>;

  { collector.reset(checkpoint) };
};

static constexpr uint8_t kNoData = 0;

using MetadataBlobSharedPtr = std::shared_ptr<std::string>;

}  // namespace PromPP::Primitives::lss_metadata