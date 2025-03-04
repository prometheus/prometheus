#pragma once

#include <cstdint>

#include <scope_exit.h>

#include "bare_bones/exception.h"
#include "bare_bones/snug_composite.h"
#include "bare_bones/stream_v_byte.h"
#include "hash.h"

namespace PromPP::Primitives::SnugComposites::Filaments {

template <template <class> class Vector>
class Symbol {
  uint32_t pos_;
  uint32_t length_;

  static constexpr bool kIsReadOnly = std::same_as<Vector<uint8_t>, BareBones::SharedSpan<uint8_t>>;

 public:
  // NOLINTNEXTLINE(readability-identifier-naming)
  class data_type : public Vector<char> {
    class Checkpoint {
      uint32_t size_;

      using SerializationMode = BareBones::SnugComposite::SerializationMode;

     public:
      explicit Checkpoint(uint32_t size) noexcept : size_(size) {}

      PROMPP_ALWAYS_INLINE uint32_t size() const { return size_; }

      template <BareBones::OutputStream S>
      void save(S& out, data_type const& data, Checkpoint const* from = nullptr) const {
        // write version
        out.put(1);

        // write mode
        SerializationMode mode = (from != nullptr) ? SerializationMode::DELTA : SerializationMode::SNAPSHOT;
        out.put(static_cast<char>(mode));

        // write pos of first seq in the portion, if we are writing delta
        uint32_t first_to_save = 0;
        if (from != nullptr) {
          first_to_save = from->size_;
          out.write(reinterpret_cast<const char*>(&first_to_save), sizeof(first_to_save));
        }

        // write  size
        uint32_t size_to_save = size_ - first_to_save;
        out.write(reinterpret_cast<char*>(&size_to_save), sizeof(size_to_save));

        // write data
        if (size_to_save > 0) {
          out.write(&data[first_to_save], size_to_save);
        }
      }

      PROMPP_ALWAYS_INLINE uint32_t save_size(data_type const&, Checkpoint const* from = nullptr) const {
        uint32_t res = 0;

        // version
        ++res;

        // mode
        ++res;

        // pos of first seq in the portion, if we are writing delta
        uint32_t first_to_save = 0;
        if (from != nullptr) {
          first_to_save = from->size_;
          res += sizeof(uint32_t);  // first index
        }

        // size
        uint32_t size_to_save = size_ - first_to_save;
        res += sizeof(uint32_t);

        // data
        res += size_to_save;

        return res;
      }
    };

   public:
    using checkpoint_type = Checkpoint;

    data_type() noexcept = default;
    data_type(const data_type&) = delete;
    template <class AnotherDataType>
      requires kIsReadOnly
    explicit data_type(const AnotherDataType& other) : Vector<char>(other) {}
    data_type(data_type&&) noexcept = delete;
    data_type& operator=(const data_type&) = delete;
    data_type& operator=(data_type&&) noexcept = delete;

    PROMPP_ALWAYS_INLINE auto checkpoint() const { return Checkpoint(this->size()); }
    PROMPP_ALWAYS_INLINE void rollback(const checkpoint_type& s) noexcept {
      assert(s.size() <= this->size());
      this->resize(s.size());
    }

    template <class InputStream>
    void load(InputStream& in) {
      // read version
      uint8_t version = in.get();
      if (version != 1) {
        throw BareBones::Exception(0x67c010edbd64e272,
                                   "Invalid stream data version (%d) for loading into data vector (Symbol::data_type), only version 1 is supported", version);
      }

      // read mode
      BareBones::SnugComposite::SerializationMode mode = static_cast<BareBones::SnugComposite::SerializationMode>(in.get());

      // read pos of first symbol in the portion, if we are reading wal
      uint32_t first_to_load_i = 0;
      if (mode == BareBones::SnugComposite::SerializationMode::DELTA) {
        in.read(reinterpret_cast<char*>(&first_to_load_i), sizeof(first_to_load_i));
      }

      if (first_to_load_i != this->size()) {
        if (mode == BareBones::SnugComposite::SerializationMode::SNAPSHOT) {
          throw BareBones::Exception(0x4c0ca0586da6da3f, "Attempt to load snapshot into non-empty data vector");
        } else if (first_to_load_i < this->size()) {
          throw BareBones::Exception(0x55cb9b02c23f7bbc, "Attempt to load segment over existing data");
        } else {
          throw BareBones::Exception(0x55cb9b02c23f7bbc, "Attempt to load incomplete data from segment, data vector length (%u) is less than segment size (%d)",
                                     this->size(), first_to_load_i);
        }
      }

      // read size
      uint32_t size_to_load;
      in.read(reinterpret_cast<char*>(&size_to_load), sizeof(size_to_load));

      // read data
      this->resize(this->size() + size_to_load);
      in.read(this->begin() + first_to_load_i, size_to_load);
    }

    PROMPP_ALWAYS_INLINE size_t remainder_size() const noexcept {
      constexpr size_t max_ui32 = std::numeric_limits<uint32_t>::max();
      assert(this->size() <= max_ui32);
      return max_ui32 - this->size();
    }
  };

  using composite_type = std::string_view;

  PROMPP_ALWAYS_INLINE Symbol() noexcept = default;

  PROMPP_ALWAYS_INLINE Symbol(data_type& data, std::string_view str) noexcept : pos_(data.size()), length_(str.length()) {
    data.push_back(str.begin(), str.end());
  }

  PROMPP_ALWAYS_INLINE composite_type composite(const data_type& data) const noexcept { return std::string_view(data.data() + pos_, length_); }

  PROMPP_ALWAYS_INLINE void validate(const data_type& data) const {
    if (pos_ + length_ > data.size()) {
      throw BareBones::Exception(0x75555f55ebe357a3, "Symbol validation error: length is out of data vector range");
    }
  }

  PROMPP_ALWAYS_INLINE uint32_t length() const noexcept { return length_; }
};

}  // namespace PromPP::Primitives::SnugComposites::Filaments

template <template <class> class Vector>
struct BareBones::IsTriviallyReallocatable<BareBones::SnugComposite::DecodingTable<PromPP::Primitives::SnugComposites::Filaments::Symbol, Vector>>
    : std::true_type {};

namespace PromPP::Primitives::SnugComposites::Filaments {

template <template <template <class> class> class SymbolsTableType, template <class> class Vector>
class LabelNameSet {
  uint32_t pos_;
  uint32_t size_;

  static constexpr bool kIsReadOnly = std::same_as<Vector<uint8_t>, BareBones::SharedSpan<uint8_t>>;

 public:
  using symbols_table_type = SymbolsTableType<Vector>;
  using symbols_ids_sequences_type = Vector<uint32_t>;

  // NOLINTNEXTLINE(readability-identifier-naming)
  struct data_type {
   private:
    class Checkpoint {
      using SerializationMode = BareBones::SnugComposite::SerializationMode;

      uint32_t size_;
      typename symbols_table_type::checkpoint_type symbols_table_checkpoint_;

     public:
      PROMPP_ALWAYS_INLINE explicit Checkpoint(data_type const& data) noexcept
          : size_(data.symbols_ids_sequences.size()), symbols_table_checkpoint_(data.symbols_table.checkpoint()) {}

      PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return size_; }

      PROMPP_ALWAYS_INLINE typename symbols_table_type::checkpoint_type symbols_table() const noexcept { return symbols_table_checkpoint_; }

      template <BareBones::OutputStream S>
      PROMPP_ALWAYS_INLINE void save(S& out, data_type const& data, Checkpoint const* from = nullptr) const {
        // write version
        out.put(1);

        // write mode
        SerializationMode mode = (from != nullptr) ? SerializationMode::DELTA : SerializationMode::SNAPSHOT;
        out.put(static_cast<char>(mode));

        // write pos of first seq in the portion, if we are writing delta
        uint32_t first_to_save = 0;
        if (from != nullptr) {
          first_to_save = from->size_;
          out.write(reinterpret_cast<const char*>(&first_to_save), sizeof(first_to_save));
        }

        // write  size
        uint32_t size_to_save = size_ - first_to_save;
        out.write(reinterpret_cast<char*>(&size_to_save), sizeof(size_to_save));

        // write data
        out.write(reinterpret_cast<const char*>(&data.symbols_ids_sequences[first_to_save]), sizeof(data.symbols_ids_sequences[first_to_save]) * size_to_save);

        // write symbols table
        if (from != nullptr) {
          symbols_table_checkpoint_.save(out, &from->symbols_table_checkpoint_);
        } else {
          symbols_table_checkpoint_.save(out);
        }
      }

      PROMPP_ALWAYS_INLINE uint32_t save_size(data_type const& data, Checkpoint const* from = nullptr) const {
        uint32_t res = 0;

        // version
        ++res;

        // mode
        ++res;

        // pos of first seq in the portion, if we are writing delta
        uint32_t first_to_save = 0;
        if (from != nullptr) {
          first_to_save = from->size_;
          res += sizeof(uint32_t);
        }

        // size
        uint32_t size_to_save = size_ - first_to_save;
        res += sizeof(uint32_t);

        // data
        res += sizeof(data.symbols_ids_sequences[first_to_save]) * size_to_save;

        // symbols table
        if (from != nullptr) {
          res += symbols_table_checkpoint_.save_size(&from->symbols_table_checkpoint_);
        } else {
          res += symbols_table_checkpoint_.save_size();
        }

        return res;
      }
    };

   public:
    symbols_table_type symbols_table;
    symbols_ids_sequences_type symbols_ids_sequences;
    data_type() noexcept = default;
    data_type(const data_type&) = delete;
    template <class AnotherDataType>
      requires kIsReadOnly
    explicit data_type(const AnotherDataType& other) : symbols_table(other.symbols_table), symbols_ids_sequences(other.symbols_ids_sequences) {}
    data_type(data_type&&) noexcept = delete;
    data_type& operator=(const data_type&) = delete;
    data_type& operator=(data_type&&) noexcept = delete;

    using checkpoint_type = Checkpoint;

    PROMPP_ALWAYS_INLINE auto checkpoint() const noexcept { return Checkpoint(*this); }

    PROMPP_ALWAYS_INLINE void rollback(const checkpoint_type& s) noexcept {
      assert(s.size() <= symbols_ids_sequences.size());
      symbols_ids_sequences.resize(s.size());
      symbols_table.rollback(s.symbols_table());
    }

    template <class InputStream>
    void load(InputStream& in) {
      // read version
      uint8_t version = in.get();
      if (version != 1) {
        throw BareBones::Exception(0xe7b943f626c40350,
                                   "Invalid stream data version (%d) for loading into LabelSetNames::data_type vector, only version 1 is supported", version);
      }

      // read mode
      BareBones::SnugComposite::SerializationMode mode = static_cast<BareBones::SnugComposite::SerializationMode>(in.get());

      // read pos of first seq in the portion, if we are reading wal
      uint32_t first_to_load_i = 0;
      if (mode == BareBones::SnugComposite::SerializationMode::DELTA) {
        in.read(reinterpret_cast<char*>(&first_to_load_i), sizeof(first_to_load_i));
      }
      if (first_to_load_i != symbols_ids_sequences.size()) {
        if (mode == BareBones::SnugComposite::SerializationMode::SNAPSHOT) {
          throw BareBones::Exception(0x484607065485b4ab, "Attempt to load snapshot into non-empty LabelSetNames data vector");
        } else if (first_to_load_i < symbols_ids_sequences.size()) {
          throw BareBones::Exception(0xc042fdcb4b149d95, "Attempt to load segment over existing LabelSetNames data");
        } else {
          throw BareBones::Exception(0x79995816e0a9690b,
                                     "Attempt to load incomplete data from segment, LabelSetNames data vector length (%u) is less than segment size (%d)",
                                     symbols_ids_sequences.size(), first_to_load_i);
        }
      }

      // read size
      uint32_t size_to_load;
      in.read(reinterpret_cast<char*>(&size_to_load), sizeof(size_to_load));

      // read data
      auto original_size = symbols_ids_sequences.size();
      auto sg1 = std::experimental::scope_fail([&]() { symbols_ids_sequences.resize(original_size); });
      symbols_ids_sequences.resize(original_size + size_to_load);
      in.read(reinterpret_cast<char*>(&symbols_ids_sequences[first_to_load_i]), sizeof(symbols_ids_sequences[first_to_load_i]) * size_to_load);

      // read symbols table
      symbols_table.load(in);
    }

    PROMPP_ALWAYS_INLINE size_t remainder_size() const noexcept {
      constexpr size_t max_ui32 = std::numeric_limits<uint32_t>::max();
      assert(this->symbols_ids_sequences.size() <= max_ui32);

      size_t remainder_for_symbols = max_ui32 - this->symbols_ids_sequences.size();

      return std::min(this->symbols_table.remainder_size(), remainder_for_symbols);
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
      return symbols_table.allocated_memory() + symbols_ids_sequences.allocated_memory();
    }
  };

  PROMPP_ALWAYS_INLINE LabelNameSet() noexcept = default;
  template <class T>
  // TODO requires is_label_name_set
  PROMPP_ALWAYS_INLINE LabelNameSet(data_type& data, const T& lns) noexcept : pos_(data.symbols_ids_sequences.size()), size_(lns.size()) {
    for (const auto& label_name : lns) {
      uint32_t smbl_id = data.symbols_table.find_or_emplace(label_name);
      data.symbols_ids_sequences.push_back(smbl_id);
    }
  }

  PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return size_; }

  // NOLINTNEXTLINE(readability-identifier-naming)
  class composite_type
      : public symbols_table_type::template Resolver<typename symbols_ids_sequences_type::const_iterator, typename symbols_ids_sequences_type::const_iterator> {
    using Base = typename symbols_table_type::template Resolver<typename symbols_ids_sequences_type::const_iterator,
                                                                typename symbols_ids_sequences_type::const_iterator>;

   public:
    using Base::Base;

    template <class T>
    PROMPP_ALWAYS_INLINE bool operator==(const T& b) const noexcept {
      return std::ranges::equal(Base::begin(), Base::end(), b.begin(), b.end());
    }

    template <class T>
    PROMPP_ALWAYS_INLINE bool operator<(const T& b) const noexcept {
      return std::ranges::lexicographical_compare(Base::begin(), Base::end(), b.begin(), b.end());
    }

    PROMPP_ALWAYS_INLINE friend size_t hash_value(const composite_type& lns) noexcept { return hash::hash_of_string_list(lns); }
  };

  PROMPP_ALWAYS_INLINE composite_type composite(const data_type& data) const noexcept {
    auto begin = data.symbols_ids_sequences.begin() + pos_;
    return composite_type(&data.symbols_table, begin, begin + size_);
  }

  PROMPP_ALWAYS_INLINE void validate(const data_type& data) const {
    if (pos_ + size_ > data.symbols_ids_sequences.size()) {
      throw BareBones::Exception(0x45e8bdc1455fd8e4, "LabelSetNames data validation error: expected LabelSetNames length is out of data vector range");
    }

    for (auto i = data.symbols_ids_sequences.begin() + pos_; i != data.symbols_ids_sequences.begin() + pos_ + size_; ++i) {
      if (*i >= data.symbols_table.size()) {
        throw BareBones::Exception(0x218410dde097cc6b, "LabelSetNames data validation error: expected LabelSetNames length is out of data symbols table range");
      }
    }
  }
};

template <template <template <class> class> class SymbolsTableType,
          template <template <class> class> class LabelNameSetsTableType,
          template <class> class Vector>
class LabelSet {
  uint32_t lns_id_;
  uint32_t pos_;

  static constexpr bool kIsReadOnly = std::same_as<Vector<uint8_t>, BareBones::SharedSpan<uint8_t>>;

 public:
  using symbols_tables_type = std::conditional_t<kIsReadOnly, BareBones::Vector<SymbolsTableType<Vector>>, Vector<std::unique_ptr<SymbolsTableType<Vector>>>>;

  using symbols_ids_sequences_type = Vector<uint8_t>;

  // NOLINTNEXTLINE(readability-identifier-naming)
  struct data_type {
   private:
    class Checkpoint {
      using SerializationMode = BareBones::SnugComposite::SerializationMode;
      using symbols_checkpoints_type = Vector<typename SymbolsTableType<Vector>::checkpoint_type>;

      const data_type* data_;
      uint32_t next_item_index_;
      uint32_t size_;
      typename LabelNameSetsTableType<Vector>::checkpoint_type label_name_sets_table_checkpoint_;
      symbols_checkpoints_type symbols_tables_checkpoints_;

     public:
      explicit PROMPP_ALWAYS_INLINE Checkpoint(const data_type& data) noexcept
          : data_(&data),
            next_item_index_(data.next_item_index_),
            size_(data.symbols_ids_sequences.size()),
            label_name_sets_table_checkpoint_(data.label_name_sets_table.checkpoint()) {
        symbols_tables_checkpoints_.reserve_and_write(data.symbols_tables.size(), [&data](auto memory, uint32_t size) {
          for (auto& symbol_table : data.symbols_tables) {
            std::construct_at(memory++, symbol_table->checkpoint());
          }
          return size;
        });
      }

      PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return size_; }

      PROMPP_ALWAYS_INLINE typename LabelNameSetsTableType<Vector>::checkpoint_type const label_name_sets() const noexcept {
        return label_name_sets_table_checkpoint_;
      }

      PROMPP_ALWAYS_INLINE Vector<typename SymbolsTableType<Vector>::checkpoint_type> symbols() const noexcept { return symbols_tables_checkpoints_; }

      template <BareBones::OutputStream S>
      PROMPP_ALWAYS_INLINE void save(S& out, data_type const& data, Checkpoint const* from = nullptr) const {
        // write version
        out.put(1);

        // write mode
        SerializationMode mode = (from != nullptr) ? SerializationMode::DELTA : SerializationMode::SNAPSHOT;
        out.put(static_cast<char>(mode));

        // write pos of first seq in the portion, if we are writing delta
        uint32_t first_to_save = 0;
        if (from != nullptr) {
          first_to_save = from->next_item_index_;
          out.write(reinterpret_cast<const char*>(&first_to_save), sizeof(first_to_save));
        }
        uint32_t first_item_index_in_ids_sequence = data_->next_item_index() - data_->symbols_ids_sequences.size();
        assert(first_to_save >= first_item_index_in_ids_sequence);

        // write  size
        uint32_t size_to_save = next_item_index_ - first_to_save;
        out.write(reinterpret_cast<char*>(&size_to_save), sizeof(size_to_save));

        // write data
        out.write(reinterpret_cast<const char*>(&data.symbols_ids_sequences[first_to_save - first_item_index_in_ids_sequence]),
                  sizeof(data.symbols_ids_sequences[0]) * size_to_save);

        // write label name sets table
        if (from != nullptr) {
          label_name_sets_table_checkpoint_.save(out, &from->label_name_sets_table_checkpoint_);
        } else {
          label_name_sets_table_checkpoint_.save(out);
        }

        // count tables, we have to write
        uint32_t number_of_symbols_tables_to_save = symbols_tables_checkpoints_.size();
        if (from != nullptr) {
          for (uint32_t i = 0; i < from->symbols_tables_checkpoints_.size(); ++i) {
            auto from_checkpoint = from->symbols_tables_checkpoints_[i];
            auto to_checkpoint = symbols_tables_checkpoints_[i];
            if ((to_checkpoint - from_checkpoint).empty()) {
              --number_of_symbols_tables_to_save;
            }
          }
        }

        // write number of symbols tables
        out.write(reinterpret_cast<char*>(&number_of_symbols_tables_to_save), sizeof(number_of_symbols_tables_to_save));
        // write symbols tables
        if (from != nullptr) {
          for (uint32_t i = 0; i < symbols_tables_checkpoints_.size(); ++i) {
            auto to_checkpoint = symbols_tables_checkpoints_[i];
            if (i >= from->symbols_tables_checkpoints_.size()) {
              // write id
              out.write(reinterpret_cast<char*>(&i), sizeof(i));
              // write symbols table
              to_checkpoint.save(out);
              continue;
            }
            auto from_checkpoint = from->symbols_tables_checkpoints_[i];
            if ((to_checkpoint - from_checkpoint).empty()) {
              continue;
            }
            // write id
            out.write(reinterpret_cast<char*>(&i), sizeof(i));
            // write symbols table
            to_checkpoint.save(out, &from_checkpoint);
          }
        } else {
          for (uint32_t i = 0; i < symbols_tables_checkpoints_.size(); ++i) {
            // write symbols table
            symbols_tables_checkpoints_[i].save(out);
          }
        }
      }

      PROMPP_ALWAYS_INLINE uint32_t save_size(data_type const& data, Checkpoint const* from = nullptr) const {
        uint32_t res = 0;

        // version
        ++res;

        // mode
        ++res;

        // pos of first seq in the portion, if we are writing wal
        uint32_t first_to_save = 0;
        if (from != nullptr) {
          first_to_save = from->next_item_index_;
          res += sizeof(uint32_t);
        }

        // size
        uint32_t size_to_save = next_item_index_ - first_to_save;
        res += sizeof(uint32_t);

        // data
        res += sizeof(data.symbols_ids_sequences[0]) * size_to_save;

        // label name sets table
        if (from != nullptr) {
          res += label_name_sets_table_checkpoint_.save_size(&from->label_name_sets_table_checkpoint_);
        } else {
          res += label_name_sets_table_checkpoint_.save_size();
        }

        // number of symbols tables
        res += sizeof(uint32_t);

        // symbols tables
        if (from != nullptr) {
          for (uint32_t i = 0; i < symbols_tables_checkpoints_.size(); ++i) {
            const typename SymbolsTableType<Vector>::checkpoint_type* from_checkpoint = nullptr;
            if (i < from->symbols_tables_checkpoints_.size()) {
              from_checkpoint = &from->symbols_tables_checkpoints_[i];
            }
            auto to_checkpoint = symbols_tables_checkpoints_[i];
            if (from_checkpoint != nullptr) {
              if ((to_checkpoint - *from_checkpoint).empty()) {
                continue;
              }
            }
            // write id
            res += sizeof(i);
            // write symbols table
            res += to_checkpoint.save_size(from_checkpoint);
          }
        } else {
          for (uint32_t i = 0; i < symbols_tables_checkpoints_.size(); ++i) {
            // write symbols table
            res += symbols_tables_checkpoints_[i].save_size();
          }
        }

        return res;
      }
    };

   public:
    using SymbolIdsCodec = BareBones::StreamVByte::Codec1234;

    symbols_tables_type symbols_tables;
    symbols_ids_sequences_type symbols_ids_sequences;
    LabelNameSetsTableType<Vector> label_name_sets_table;
    uint32_t next_item_index_{};
    uint32_t shrinked_size_{};
    data_type() noexcept = default;
    data_type(const data_type&) = delete;

    template <class AnotherDataType>
      requires kIsReadOnly
    explicit data_type(const AnotherDataType& other)
        : symbols_ids_sequences(other.symbols_ids_sequences),
          label_name_sets_table(other.label_name_sets_table),
          next_item_index_(other.next_item_index_),
          shrinked_size_(other.shrinked_size_) {
      symbols_tables.reserve_and_write(other.symbols_tables.size(), [&other](auto memory, uint32_t size) {
        for (auto& symbol_table : other.symbols_tables) {
          std::construct_at(memory++, *symbol_table);
        }
        return size;
      });
    }

    data_type(data_type&&) noexcept = delete;
    data_type& operator=(const data_type&) = delete;
    data_type& operator=(data_type&&) noexcept = delete;

    using checkpoint_type = Checkpoint;

    PROMPP_ALWAYS_INLINE void shrink_to(uint32_t size) {
      assert(size <= symbols_ids_sequences.size());

      shrinked_size_ += symbols_ids_sequences.size() - size;
      symbols_ids_sequences.resize(size);
      symbols_ids_sequences.shrink_to_fit();
    }

    PROMPP_ALWAYS_INLINE auto checkpoint() const noexcept { return Checkpoint(*this); }

    PROMPP_ALWAYS_INLINE void rollback(const checkpoint_type& s) noexcept {
      assert(s.size() <= symbols_ids_sequences.size());
      symbols_ids_sequences.resize(s.size());

      label_name_sets_table.rollback(s.label_name_sets());

      auto symbols_tables_checkpoints = s.symbols();
      assert(symbols_tables_checkpoints.size() <= symbols_tables.size());
      for (uint32_t i = 0; i != symbols_tables_checkpoints.size(); ++i) {
        symbols_tables[i]->rollback(symbols_tables_checkpoints[i]);
      }
      symbols_tables.resize(symbols_tables_checkpoints.size());
    }

    template <class InputStream>
    void load(InputStream& in) {
      // read version
      const uint8_t version = in.get();
      if (version != 1) {
        throw BareBones::Exception(0x7524f0b0ab963554, "Invalid stream data version (%d) for loading LabelSets into data vector, only version 1 is supported",
                                   version);
      }

      // read mode
      const auto mode = static_cast<BareBones::SnugComposite::SerializationMode>(in.get());

      // read pos of first seq in the portion, if we are reading wal
      uint32_t first_to_load_i = 0;
      if (mode == BareBones::SnugComposite::SerializationMode::DELTA) {
        in.read(reinterpret_cast<char*>(&first_to_load_i), sizeof(first_to_load_i));
      }
      if (first_to_load_i != next_item_index_) {
        if (mode == BareBones::SnugComposite::SerializationMode::SNAPSHOT) {
          throw BareBones::Exception(0xefdd57cef4b89243, "Attempt to load snapshot into non-empty LabelSets data vector");
        } else if (first_to_load_i < symbols_ids_sequences.size()) {
          throw BareBones::Exception(0xfead3117c5a549bd, "Attempt to load segment over existing LabelSets data");
        } else {
          throw BareBones::Exception(0xbb996a8ffbcbb53b,
                                     "Attempt to load incomplete data from segment, LabelSets data vector length (%u) is less than segment size (%d)",
                                     symbols_ids_sequences.size(), first_to_load_i);
        }
      }

      // read size
      uint32_t size_to_load;
      in.read(reinterpret_cast<char*>(&size_to_load), sizeof(size_to_load));

      // read data
      auto sg1 = std::experimental::scope_fail([original_size = symbols_ids_sequences.size(), this] { symbols_ids_sequences.resize(original_size); });

      symbols_ids_sequences.reserve_and_write(size_to_load + sizeof(SymbolIdsCodec::value_type), [&in, size_to_load](uint8_t* buffer, uint32_t) {
        in.read(reinterpret_cast<char*>(buffer), size_to_load * sizeof(symbols_ids_sequences[first_to_load_i]));
        return size_to_load;
      });
      next_item_index_ += size_to_load;

      // read label name sets table
      auto label_name_sets_table_checkpoint = label_name_sets_table.checkpoint();
      auto sg2 = std::experimental::scope_fail([&]() { label_name_sets_table.rollback(label_name_sets_table_checkpoint); });
      label_name_sets_table.load(in);

      // read number of tables
      uint32_t number_of_symbols_tables_to_load;
      in.read(reinterpret_cast<char*>(&number_of_symbols_tables_to_load), sizeof(number_of_symbols_tables_to_load));

      // read tables
      auto original_symbols_tables_size = symbols_tables.size();
      BareBones::Vector<std::pair<uint32_t, typename SymbolsTableType<Vector>::checkpoint_type>> symbols_tables_checkpoints;
      auto sg3 = std::experimental::scope_fail([&]() {
        for (const auto& [id, checkpoint] : symbols_tables_checkpoints) {
          symbols_tables[id]->rollback(checkpoint);
        }
        symbols_tables.resize(original_symbols_tables_size);
      });
      for (uint32_t i = 0; i != number_of_symbols_tables_to_load; ++i) {
        // read id
        uint32_t id;

        if (mode == BareBones::SnugComposite::SerializationMode::DELTA) {
          in.read(reinterpret_cast<char*>(&id), sizeof(id));
        } else {
          id = i;
        }

        // resize, if needed
        if (id >= symbols_tables.size()) {
          const auto number_of_tables_stil_left_to_load = number_of_symbols_tables_to_load - i;
          auto size_will_be_at_least = id + number_of_tables_stil_left_to_load;

          symbols_tables.reserve(size_will_be_at_least);
          for (auto j = symbols_tables.size(); j != size_will_be_at_least; ++j) {
            symbols_tables.emplace_back(std::make_unique<SymbolsTableType<Vector>>());
          }

          // just to be 100% sure
          assert(id < symbols_tables.size());
        }

        // read symbols table
        if (mode == BareBones::SnugComposite::SerializationMode::DELTA && id < original_symbols_tables_size)
          symbols_tables_checkpoints.emplace_back(id, symbols_tables[id]->checkpoint());
        symbols_tables[id]->load(in);
      }

      // just to be 100% sure
      assert(label_name_sets_table.data().symbols_table.size() == symbols_tables.size());
    }

    // it drains before the maximum available symbols count would be exceeded.
    PROMPP_ALWAYS_INLINE size_t remainder_size() const noexcept {
      constexpr size_t max_ui32 = std::numeric_limits<uint32_t>::max();

      assert(this->symbols_ids_sequences.size() <= max_ui32);

      size_t remainder_for_label_sets_table = this->label_name_sets_table.remainder_size();
      size_t remainder_for_symbols_ids_sequences = max_ui32 - this->symbols_ids_sequences.size();
      size_t remainder_for_symbol_table = std::numeric_limits<uint32_t>::max();
      for (const auto& table : this->symbols_tables) {
        if (size_t n = table->remainder_size(); n < remainder_for_symbol_table) {
          remainder_for_symbol_table = n;
        }
      }
      return std::min({remainder_for_label_sets_table, remainder_for_symbols_ids_sequences, remainder_for_symbol_table});
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
      return symbols_tables.allocated_memory() + symbols_ids_sequences.allocated_memory() + label_name_sets_table.allocated_memory();
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t next_item_index() const noexcept { return next_item_index_; }
  };

  PROMPP_ALWAYS_INLINE LabelSet() noexcept = default;

  // FIXME inline of this function causes 30ns lost in indexing performance
  template <class T>
  // TODO requires is_label_set
  LabelSet(data_type& data, const T& label_set) noexcept : pos_(data.symbols_ids_sequences.size() + data.shrinked_size_) {
    lns_id_ = data.label_name_sets_table.find_or_emplace(label_set.names());

    // resize, if there are new symbols (in lns table)
    data.symbols_tables.reserve(data.label_name_sets_table.data().symbols_table.size());
    for (auto i = data.symbols_tables.size(); i < data.label_name_sets_table.data().symbols_table.size(); ++i) {
      data.symbols_tables.emplace_back(std::make_unique<SymbolsTableType<Vector>>());
    }

    auto lns = data.label_name_sets_table[lns_id_];
    auto lns_i = lns.begin();
    auto size_before = data.symbols_ids_sequences.size();
    auto i = BareBones::StreamVByte::back_inserter<typename data_type::SymbolIdsCodec>(data.symbols_ids_sequences, lns.size());
    for (auto [_, label_value] : label_set) {
      *i++ = data.symbols_tables[lns_i.id()]->find_or_emplace(label_value);
      ++lns_i;
    }

    data.next_item_index_ += data.symbols_ids_sequences.size() - size_before;
  }

  // NOLINTNEXTLINE(readability-identifier-naming)
  class composite_type {
    using label_name_set_type = typename LabelNameSetsTableType<Vector>::value_type;
    using values_iterator_type =
        BareBones::StreamVByte::DecodeIterator<typename data_type::SymbolIdsCodec, typename symbols_ids_sequences_type::const_iterator>;
    using values_iterator_sentinel_type = BareBones::StreamVByte::DecodeIteratorSentinel;

    label_name_set_type label_name_set_;
    const data_type* data_;
    values_iterator_type values_begin_;
    values_iterator_sentinel_type values_end_;

   public:
    PROMPP_ALWAYS_INLINE explicit composite_type(const data_type* data = nullptr,
                                                 label_name_set_type label_name_set = label_name_set_type(),
                                                 values_iterator_type values_begin = values_iterator_type(),
                                                 values_iterator_sentinel_type values_end = values_iterator_sentinel_type()) noexcept
        : label_name_set_(label_name_set), data_(data), values_begin_(values_begin), values_end_(values_end) {}

    using value_type = std::pair<typename label_name_set_type::value_type, typename Symbol<Vector>::composite_type>;

    PROMPP_ALWAYS_INLINE const label_name_set_type& names() const noexcept { return label_name_set_; }

    PROMPP_ALWAYS_INLINE auto size() const noexcept { return label_name_set_.size(); }

    template <class LabelNameSetIteratorType, class ValuesIteratorType>
    class Iterator {
      LabelNameSetIteratorType lnsi_;
      ValuesIteratorType vi_;
      const data_type* data_;

      friend class composite_type;

     public:
      using iterator_category = std::forward_iterator_tag;
      using value_type = composite_type::value_type;
      using difference_type = std::ptrdiff_t;

      PROMPP_ALWAYS_INLINE explicit Iterator(const data_type* data = 0,
                                             LabelNameSetIteratorType lnsi = LabelNameSetIteratorType(),
                                             ValuesIteratorType vi = ValuesIteratorType()) noexcept
          : lnsi_(lnsi), vi_(vi), data_(data) {}

      PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
        ++lnsi_;
        ++vi_;
        return *this;
      }

      PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
        Iterator retval = *this;
        ++(*this);
        return retval;
      }

      template <class OtherIteratorType>
      PROMPP_ALWAYS_INLINE bool operator==(const OtherIteratorType& other) const noexcept {
        return lnsi_ == other.lnsi_ && vi_ == other.vi_;
      }

      PROMPP_ALWAYS_INLINE value_type operator*() const noexcept {
        if constexpr (BareBones::concepts::is_dereferenceable<decltype(data_->symbols_tables[lnsi_.id()])>) {
          const auto& smbl_tbl = *data_->symbols_tables[lnsi_.id()];
          return make_pair(*lnsi_, smbl_tbl[*vi_]);
        } else {
          const auto& smbl_tbl = data_->symbols_tables[lnsi_.id()];
          return make_pair(*lnsi_, smbl_tbl[*vi_]);
        }
      }

      PROMPP_ALWAYS_INLINE uint32_t name_id() const noexcept { return lnsi_.id(); }

      PROMPP_ALWAYS_INLINE uint32_t value_id() const noexcept { return *vi_; }
    };

    PROMPP_ALWAYS_INLINE auto begin() const noexcept {
      return Iterator<decltype(label_name_set_.begin()), decltype(values_begin_)>(data_, label_name_set_.begin(), values_begin_);
    }

    PROMPP_ALWAYS_INLINE auto end() const noexcept {
      return Iterator<decltype(label_name_set_.end()), decltype(values_end_)>(data_, label_name_set_.end(), values_end_);
    }

    template <class T>
    PROMPP_ALWAYS_INLINE bool operator==(const T& b) const noexcept {
      return std::ranges::equal(begin(), end(), b.begin(), b.end(), [](const auto& a, const auto& b) { return a == b; });
    }

    template <class T>
    PROMPP_ALWAYS_INLINE bool operator<(const T& b) const noexcept {
      return std::ranges::lexicographical_compare(begin(), end(), b.begin(), b.end(), [](const auto& a, const auto& b) { return a < b; });
    }

    PROMPP_ALWAYS_INLINE friend size_t hash_value(const composite_type& ls) noexcept { return hash::hash_of_label_set(ls); }
  };

  PROMPP_ALWAYS_INLINE composite_type composite(const data_type& data) const noexcept {
    auto lns = data.label_name_sets_table[lns_id_];

    auto [values_begin, values_end] =
        BareBones::StreamVByte::decoder<typename data_type::SymbolIdsCodec>(data.symbols_ids_sequences.begin() + pos_ - data.shrinked_size_, lns.size());

    return composite_type(&data, std::move(lns), std::move(values_begin), std::move(values_end));
  }

  PROMPP_ALWAYS_INLINE void validate(const data_type& data) const {
    if (lns_id_ >= data.label_name_sets_table.size()) {
      throw BareBones::Exception(0x48dd6c9d357d3a7e, "LabelSets data validation error: expected LabelSets length is out of label name sets table vector range");
    }

    const auto& lns = data.label_name_sets_table[lns_id_];

    // check that streamvbyte keys are in range
    auto keys_size = BareBones::StreamVByte::keys_size(lns.size());
    if (pos_ - data.shrinked_size_ + keys_size > data.symbols_ids_sequences.size()) {
      throw BareBones::Exception(0x22f5a82dd120e0e7, "LabelSets data validation error: expected LabelSets keys length is out of data symbols vector range");
    }

    // check that streamvbyte data is in range
    auto data_size = BareBones::StreamVByte::decode_data_size<BareBones::StreamVByte::Codec1234>(
        lns.size(), data.symbols_ids_sequences.begin() + pos_ - data.shrinked_size_);
    if (pos_ - data.shrinked_size_ + keys_size + data_size > data.symbols_ids_sequences.size()) {
      throw BareBones::Exception(0xd02e54dac8e1d328, "LabelSets data validation error: expected LabelSets values length is out of data symbols vector range");
    }

    // check that all symbols are in range
    auto [values_begin, values_end] =
        BareBones::StreamVByte::decoder<BareBones::StreamVByte::Codec1234>(data.symbols_ids_sequences.begin() + pos_ - data.shrinked_size_, lns.size());
    for (auto i = lns.begin(); i != lns.end(); ++i) {
      if (*values_begin++ >= data.symbols_tables[i.id()]->size()) {
        throw BareBones::Exception(0x0f0c520ad6285f15,
                                   "LabelSets data validation error: expected LabelSets symbols length is out of data symbols vector range");
      }
    }
  }
};

}  // namespace PromPP::Primitives::SnugComposites::Filaments
