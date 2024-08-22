#pragma once

#include "bare_bones/streams.h"
#include "primitives/snug_composites.h"
#include "types.h"

namespace PromPP::Primitives::lss_metadata {

template <class SymbolsTable, ChangesCollectorInterface ChangesCollector>
class Storage {
 private:
  using SymbolsCheckpoint = typename SymbolsTable::checkpoint_type;
  using SerializationMode = BareBones::SnugComposite::SerializationMode;

 public:
  class DeltaWriter {
   public:
    explicit DeltaWriter(const Storage& storage) : storage_(storage) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE static uint8_t version() noexcept { return 1; }

    void write(std::ostream& stream) const {
      if (storage_.changes_.count() == 0) {
        stream.put(kNoData);
      } else {
        stream.put(version());
        write_new_symbols(stream);
        write_changes(stream);
      }
    }

    PROMPP_ALWAYS_INLINE friend std::ostream& operator<<(std::ostream& stream, const DeltaWriter& delta) {
      delta.write(stream);
      return stream;
    }

   private:
    const Storage& storage_;

    [[nodiscard]] PROMPP_ALWAYS_INLINE SerializationMode mode() const noexcept {
      return storage_.changes_.symbols_table_checkpoint().symbols_count == 0 ? SerializationMode::SNAPSHOT : SerializationMode::DELTA;
    }

    void write_new_symbols(std::ostream& stream) const { stream << storage_.symbols_.checkpoint() - get_previous_symbols_checkpoint(); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE SymbolsCheckpoint get_previous_symbols_checkpoint() const noexcept {
      auto& checkpoint = storage_.changes_.symbols_table_checkpoint();
      return {storage_.symbols_, checkpoint.symbols_count, checkpoint.symbols_count,
              typename SymbolsTable::data_type::checkpoint_type{checkpoint.symbols_data_size}};
    }

    void write_changes(std::ostream& stream) const {
      const ChangesCollectorInterface auto& changes = storage_.changes_;
      auto changes_count = changes.count();
      stream.write(reinterpret_cast<const char*>(&changes_count), sizeof(changes_count));

      if (changes_count == 0) {
        return;
      }

      write_metadata_delta(stream);
    }

    void write_metadata_delta(std::ostream& stream) const {
      for (auto name_id : storage_.changes_) {
        stream.write(reinterpret_cast<const char*>(&name_id), sizeof(name_id));

        auto& metadata = storage_.series_metadata_[name_id];
        stream.write(reinterpret_cast<const char*>(&metadata), sizeof(metadata));
      }
    }
  };

  class DeltaLoader {
   public:
    explicit DeltaLoader(Storage& storage) : storage_(storage) {}

    void load(std::istream& stream) {
      if (uint8_t version = stream.get(); version == kNoData) {
        return;
      } else if (version != 1) {
        throw BareBones::Exception(0xe46562d01e29e691, "Invalid lss metadata version %d got from input stream, only version 1 is supported", version);
      }

      storage_.symbols_.load(stream);

      BareBones::ExceptionsGuard exceptions_guard(stream, BareBones::ExceptionsGuard::kAllExceptions);

      uint32_t changes_count{};
      stream.read(reinterpret_cast<char*>(&changes_count), sizeof(changes_count));
      if (changes_count == 0) {
        return;
      }

      load_changes(stream, changes_count);
    }

   private:
    Storage& storage_;

    void load_changes(std::istream& stream, uint32_t count) {
      for (uint32_t i = 0; i < count; ++i) {
        uint32_t name_id{};
        stream.read(reinterpret_cast<char*>(&name_id), sizeof(name_id));

        if (name_id >= storage_.series_metadata_.size()) {
          storage_.series_metadata_.resize(name_id + 1);
        }

        auto& metadata = storage_.series_metadata_[name_id];
        stream.read(reinterpret_cast<char*>(&metadata), sizeof(metadata));

        storage_.changes_.add(name_id);
      }
    }
  };

  void add(uint32_t name_id, const Metadata& metadata)
    requires BareBones::SnugComposite::have_find_or_emplace<SymbolsTable, Metadata>
  {
    if (name_id >= series_metadata_.size()) {
      series_metadata_.resize(name_id + 1);
    }

    if (update(metadata, series_metadata_[name_id])) {
      changes_.add(name_id);
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Metadata get(uint32_t name_id) const noexcept {
    return series_metadata_.size() > name_id ? get(series_metadata_[name_id]) : Metadata{};
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t allocated_memory() const noexcept {
    return symbols_.allocated_memory() + series_metadata_.allocated_memory() + changes_.allocated_memory();
  }

  PROMPP_ALWAYS_INLINE void reset_changes() noexcept {
    changes_.reset(SymbolsTableCheckpoint{.symbols_count = symbols_.size(), .symbols_data_size = static_cast<uint32_t>(symbols_.data().size())});
  }

  PROMPP_ALWAYS_INLINE void write_changes(std::ostream& stream) const { stream << DeltaWriter(*this); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE MetadataBlobSharedPtr get_changes() const {
    std::ostringstream stream;
    write_changes(stream);
    return std::make_unique<std::string>(std::move(stream).str());
  }

  PROMPP_ALWAYS_INLINE friend std::istream& operator>>(std::istream& stream, Storage& metadata) {
    DeltaLoader{metadata}.load(stream);
    return stream;
  }

 private:
  struct PROMPP_ATTRIBUTE_PACKED MetadataIds {
    static constexpr uint32_t kInvalidSymbolId = std::numeric_limits<uint32_t>::max();

    uint32_t help{kInvalidSymbolId};
    uint32_t type{kInvalidSymbolId};
    uint32_t unit{kInvalidSymbolId};

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool type_is_filled() const noexcept { return type != kInvalidSymbolId; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE bool unit_is_filled() const noexcept { return unit != kInvalidSymbolId; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE bool help_is_filled() const noexcept { return help != kInvalidSymbolId; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool help_can_be_updated(uint32_t new_help) const noexcept { return !help_is_filled() || new_help > help; }
  };

  SymbolsTable symbols_;
  BareBones::Vector<MetadataIds> series_metadata_;
  [[no_unique_address]] ChangesCollector changes_;

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool update(const Metadata& metadata, MetadataIds& metadata_ids) noexcept
    requires BareBones::SnugComposite::have_find_or_emplace<SymbolsTable, Metadata>
  {
    bool have_changes{};

    if (auto help_id = symbols_.find_or_emplace(metadata.help); metadata_ids.help_can_be_updated(help_id)) {
      metadata_ids.help = help_id;
      have_changes = true;
    }

    if (!metadata_ids.type_is_filled()) {
      metadata_ids.type = symbols_.find_or_emplace(metadata.type);
      have_changes = true;
    }

    if (!metadata_ids.unit_is_filled()) {
      metadata_ids.unit = symbols_.find_or_emplace(metadata.unit);
      have_changes = true;
    }

    return have_changes;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Metadata get(const MetadataIds& metadata_ids) const noexcept {
    return {
        .help = symbols_[metadata_ids.help],
        .type = symbols_[metadata_ids.type],
        .unit = symbols_[metadata_ids.unit],
    };
  }
};

template <class ChangesCollector>
using MutableStorage = Storage<PromPP::Primitives::SnugComposites::Symbol::EncodingBimap, ChangesCollector>;

template <class ChangesCollector>
using ImmutableStorage = Storage<PromPP::Primitives::SnugComposites::Symbol::DecodingTable, ChangesCollector>;

}  // namespace PromPP::Primitives::lss_metadata