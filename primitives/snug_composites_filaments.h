#pragma once

#include <cstdint>

#include <scope_exit.h>
#define XXH_INLINE_ALL
#include "third_party/xxhash/xxhash.h"

#include "bare_bones/exception.h"
#include "bare_bones/snug_composite.h"
#include "bare_bones/stream_v_byte.h"
#include "bare_bones/vector.h"

namespace PromPP {
namespace Primitives {
namespace SnugComposites {
namespace Filaments {
class Symbol {
  uint32_t pos_;
  uint32_t length_;

 public:
  // NOLINTNEXTLINE(readability-identifier-naming)
  class data_type : public BareBones::Vector<char> {
    class Checkpoint {
      uint32_t size_;

      using SerializationMode = BareBones::SnugComposite::SerializationMode;

     public:
      explicit Checkpoint(uint32_t size) noexcept : size_(size) {}

      inline __attribute__((always_inline)) uint32_t size() const { return size_; }

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
        out.write(&data[first_to_save], size_to_save);
      }

      inline __attribute__((always_inline)) uint32_t save_size(data_type const& _data, Checkpoint const* from = nullptr) const {
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
    inline __attribute__((always_inline)) auto checkpoint() const { return Checkpoint(size()); }
    inline __attribute__((always_inline)) void rollback(const checkpoint_type& s) noexcept {
      assert(s.size() <= size());
      resize(s.size());
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

      if (first_to_load_i != size()) {
        if (mode == BareBones::SnugComposite::SerializationMode::SNAPSHOT) {
          throw BareBones::Exception(0x4c0ca0586da6da3f, "Attempt to load snapshot into non-empty data vector");
        } else if (first_to_load_i < size()) {
          throw BareBones::Exception(0x55cb9b02c23f7bbc, "Attempt to load segment over existing data");
        } else {
          throw BareBones::Exception(0x55cb9b02c23f7bbc,
                                     "Attempt to load incomplete data from segment, data vector length (%zd) is less than segment size (%d)", size(),
                                     first_to_load_i);
        }
      }

      // read size
      uint32_t size_to_load;
      in.read(reinterpret_cast<char*>(&size_to_load), sizeof(size_to_load));

      // read data
      resize(size() + size_to_load);
      in.read(begin() + first_to_load_i, size_to_load);
    }
  };

  using composite_type = std::string_view;

  inline __attribute__((always_inline)) Symbol() noexcept = default;

  inline __attribute__((always_inline)) Symbol(data_type& data, std::string_view str) noexcept : pos_(data.size()), length_(str.length()) {
    data.push_back(str.begin(), str.end());
  }

  inline __attribute__((always_inline)) composite_type composite(const data_type& data) const noexcept { return std::string_view(data.data() + pos_, length_); }

  inline __attribute__((always_inline)) void validate(const data_type& data) const {
    if (pos_ + length_ > data.size()) {
      throw BareBones::Exception(0x75555f55ebe357a3, "Symbol validation error: length is out of data vector range");
    }
  }

  inline __attribute__((always_inline)) const uint32_t length() const noexcept { return length_; }
};

template <class SymbolsTableType>
class LabelNameSet {
  uint32_t pos_;
  uint32_t size_;

 public:
  using symbols_table_type = SymbolsTableType;
  using symbols_ids_sequences_type = BareBones::Vector<uint32_t>;

  // NOLINTNEXTLINE(readability-identifier-naming)
  struct data_type {
   private:
    class Checkpoint {
      using SerializationMode = BareBones::SnugComposite::SerializationMode;

      uint32_t size_;
      typename symbols_table_type::checkpoint_type symbols_table_checkpoint_;

     public:
      inline __attribute__((always_inline)) explicit Checkpoint(data_type const& data) noexcept
          : size_(data.symbols_ids_sequences.size()), symbols_table_checkpoint_(data.symbols_table.checkpoint()) {}

      inline __attribute__((always_inline)) uint32_t size() const noexcept { return size_; }

      inline __attribute__((always_inline)) typename symbols_table_type::checkpoint_type symbols_table() const noexcept { return symbols_table_checkpoint_; }

      template <BareBones::OutputStream S>
      inline __attribute__((always_inline)) void save(S& out, data_type const& data, Checkpoint const* from = nullptr) const {
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

      inline __attribute__((always_inline)) uint32_t save_size(data_type const& data, Checkpoint const* from = nullptr) const {
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
    data_type(data_type&&) = delete;
    data_type& operator=(const data_type&) = delete;
    data_type& operator=(data_type&&) = delete;

    using checkpoint_type = Checkpoint;

    inline __attribute__((always_inline)) auto checkpoint() const noexcept { return Checkpoint(*this); }

    inline __attribute__((always_inline)) void rollback(const checkpoint_type& s) noexcept {
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
                                     "Attempt to load incomplete data from segment, LabelSetNames data vector length (%zd) is less than segment size (%d)",
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
  };

  inline __attribute__((always_inline)) LabelNameSet() noexcept = default;
  template <class T>
  // TODO requires is_label_name_set
  inline __attribute__((always_inline)) LabelNameSet(data_type& data, const T& lns) noexcept : pos_(data.symbols_ids_sequences.size()), size_(lns.size()) {
    for (auto label_name : lns) {
      uint32_t smbl_id = data.symbols_table.find_or_emplace(label_name);
      data.symbols_ids_sequences.push_back(smbl_id);
    }
  }

  inline __attribute__((always_inline)) uint32_t size() const noexcept { return size_; }

  // NOLINTNEXTLINE(readability-identifier-naming)
  class composite_type : public symbols_table_type::template Resolver<symbols_ids_sequences_type::const_iterator, symbols_ids_sequences_type::const_iterator> {
    using Base = typename symbols_table_type::template Resolver<symbols_ids_sequences_type::const_iterator, symbols_ids_sequences_type::const_iterator>;

   public:
    using Base::Base;

    template <class T>
    inline __attribute__((always_inline)) bool operator==(const T& b) const noexcept {
      return std::ranges::equal(Base::begin(), Base::end(), b.begin(), b.end());
    }

    template <class T>
    inline __attribute__((always_inline)) bool operator<(const T& b) const noexcept {
      return std::ranges::lexicographical_compare(Base::begin(), Base::end(), b.begin(), b.end());
    }

    inline __attribute__((always_inline)) friend size_t hash_value(const composite_type& lns) noexcept {
      size_t res = 0;
      for (const auto& label_name : lns) {
        res = XXH3_64bits_withSeed(label_name.data(), label_name.size(), res);
      }
      return res;
    }
  };

  inline __attribute__((always_inline)) composite_type composite(const data_type& data) const noexcept {
    auto begin = data.symbols_ids_sequences.begin() + pos_;
    return composite_type(&data.symbols_table, begin, begin + size_);
  }

  inline __attribute__((always_inline)) void validate(const data_type& data) const {
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

template <class SymbolsTableType, class LabelNameSetsTableType>
class LabelSet {
  uint32_t lns_id_;
  uint32_t pos_;

 public:
  using symbols_table_type = SymbolsTableType;
  using symbols_tables_type = BareBones::Vector<std::unique_ptr<symbols_table_type>>;
  using symbols_ids_sequences_type = BareBones::Vector<uint8_t>;
  using label_name_sets_table_type = LabelNameSetsTableType;

  // NOLINTNEXTLINE(readability-identifier-naming)
  struct data_type {
   private:
    class Checkpoint {
      using SerializationMode = BareBones::SnugComposite::SerializationMode;
      using symbols_checkpoints_type = BareBones::Vector<typename symbols_table_type::checkpoint_type>;

      uint32_t size_;
      typename label_name_sets_table_type::checkpoint_type label_name_sets_table_checkpoint_;
      symbols_checkpoints_type symbols_tables_checkpoints_;

     public:
      inline __attribute__((always_inline)) Checkpoint(data_type const& data) noexcept
          : size_(data.symbols_ids_sequences.size()), label_name_sets_table_checkpoint_(data.label_name_sets_table.checkpoint()) {
        for (const auto& symbols_table : data.symbols_tables) {
          symbols_tables_checkpoints_.emplace_back(symbols_table->checkpoint());
        }
      }

      inline __attribute__((always_inline)) uint32_t size() const noexcept { return size_; }

      inline __attribute__((always_inline)) typename label_name_sets_table_type::checkpoint_type const label_name_sets() const noexcept {
        return label_name_sets_table_checkpoint_;
      }

      inline __attribute__((always_inline)) const BareBones::Vector<typename symbols_table_type::checkpoint_type> symbols() const noexcept {
        return symbols_tables_checkpoints_;
      }

      template <BareBones::OutputStream S>
      inline __attribute__((always_inline)) void save(S& out, data_type const& data, Checkpoint const* from = nullptr) const {
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

      inline __attribute__((always_inline)) uint32_t save_size(data_type const& data, Checkpoint const* from = nullptr) const {
        uint32_t res = 0;

        // version
        ++res;

        // mode
        ++res;

        // pos of first seq in the portion, if we are writing wal
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
            const typename symbols_table_type::checkpoint_type* from_checkpoint = nullptr;
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
    symbols_tables_type symbols_tables;
    symbols_ids_sequences_type symbols_ids_sequences;
    label_name_sets_table_type label_name_sets_table;
    data_type() noexcept = default;
    data_type(const data_type&) = delete;
    data_type(data_type&&) = delete;
    data_type& operator=(const data_type&) = delete;
    data_type& operator=(data_type&&) = delete;

    using checkpoint_type = Checkpoint;
    inline __attribute__((always_inline)) auto checkpoint() const noexcept { return Checkpoint(*this); }

    inline __attribute__((always_inline)) void rollback(const checkpoint_type& s) noexcept {
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
      uint8_t version = in.get();
      if (version != 1) {
        throw BareBones::Exception(0x7524f0b0ab963554, "Invalid stream data version (%d) for loading LabelSets into data vector, only version 1 is supported",
                                   version);
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
          throw BareBones::Exception(0xefdd57cef4b89243, "Attempt to load snapshot into non-empty LabelSets data vector");
        } else if (first_to_load_i < symbols_ids_sequences.size()) {
          throw BareBones::Exception(0xfead3117c5a549bd, "Attempt to load segment over existing LabelSets data");
        } else {
          throw BareBones::Exception(0xbb996a8ffbcbb53b,
                                     "Attempt to load incomplete data from segment, LabelSets data vector length (%zd) is less than segment size (%d)",
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
      in.read(reinterpret_cast<char*>(&symbols_ids_sequences[first_to_load_i]), size_to_load * sizeof(symbols_ids_sequences[first_to_load_i]));

      // read label name sets table
      auto label_name_sets_table_checkpoint = label_name_sets_table.checkpoint();
      auto sg2 = std::experimental::scope_fail([&]() { label_name_sets_table.rollback(label_name_sets_table_checkpoint); });
      label_name_sets_table.load(in);

      // read number of tables
      uint32_t number_of_symbols_tables_to_load;
      in.read(reinterpret_cast<char*>(&number_of_symbols_tables_to_load), sizeof(number_of_symbols_tables_to_load));

      // read tables
      auto original_symbols_tables_size = symbols_tables.size();
      BareBones::Vector<std::pair<uint32_t, typename LabelSet::symbols_table_type::checkpoint_type>> symbols_tables_checkpoints;
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
          auto number_of_tables_stil_left_to_load = number_of_symbols_tables_to_load - i;
          auto size_will_be_at_least = id + number_of_tables_stil_left_to_load;

          symbols_tables.reserve(size_will_be_at_least);
          for (auto j = symbols_tables.size(); j != size_will_be_at_least; ++j) {
            symbols_tables.emplace_back(std::make_unique<symbols_table_type>());
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
  };

  inline __attribute__((always_inline)) LabelSet() noexcept = default;

  // FIXME inline of this function causes 30ns lost in indexing performance
  template <class T>
  // TODO requires is_label_set
  LabelSet(data_type& data, const T& label_set) noexcept : pos_(data.symbols_ids_sequences.size()) {
    lns_id_ = data.label_name_sets_table.find_or_emplace(label_set.names());

    // resize, if there are new symbols (in lns table)
    for (auto i = data.symbols_tables.size(); i < data.label_name_sets_table.data().symbols_table.size(); ++i) {
      data.symbols_tables.emplace_back(new symbols_table_type());
    }
    assert(data.symbols_tables.size() == data.label_name_sets_table.data().symbols_table.size());

    auto lns = data.label_name_sets_table[lns_id_];
    auto lns_i = lns.begin();
    auto i = BareBones::StreamVByte::back_inserter<BareBones::StreamVByte::Codec1234>(data.symbols_ids_sequences, lns.size());
    for (auto [_, label_value] : label_set) {
      *i++ = data.symbols_tables[lns_i.id()]->find_or_emplace(label_value);
      lns_i++;
    }
  }

  // NOLINTNEXTLINE(readability-identifier-naming)
  class composite_type {
    using label_name_set_type = typename label_name_sets_table_type::value_type;
    using values_iterator_type = BareBones::StreamVByte::DecodeIterator<BareBones::StreamVByte::Codec1234, symbols_ids_sequences_type::const_iterator>;
    using values_iterator_sentinel_type = BareBones::StreamVByte::DecodeIteratorSentinel;

    label_name_set_type label_name_set_;
    const data_type* data_;
    values_iterator_type values_begin_;
    values_iterator_sentinel_type values_end_;

   public:
    inline __attribute__((always_inline)) explicit composite_type(const data_type* data = nullptr,
                                                                  label_name_set_type label_name_set = label_name_set_type(),
                                                                  values_iterator_type values_begin = values_iterator_type(),
                                                                  values_iterator_sentinel_type values_end = values_iterator_sentinel_type()) noexcept
        : label_name_set_(label_name_set), data_(data), values_begin_(values_begin), values_end_(values_end) {}

    using value_type = std::pair<typename label_name_set_type::value_type, Symbol::composite_type>;

    inline __attribute__((always_inline)) const label_name_set_type& names() const noexcept { return label_name_set_; }

    inline __attribute__((always_inline)) const auto size() const noexcept { return label_name_set_.size(); }

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

      inline __attribute__((always_inline)) explicit Iterator(const data_type* data = 0,
                                                              LabelNameSetIteratorType lnsi = LabelNameSetIteratorType(),
                                                              ValuesIteratorType vi = ValuesIteratorType()) noexcept
          : lnsi_(lnsi), vi_(vi), data_(data) {}

      inline __attribute__((always_inline)) Iterator& operator++() noexcept {
        ++lnsi_;
        ++vi_;
        return *this;
      }

      inline __attribute__((always_inline)) Iterator operator++(int) noexcept {
        Iterator retval = *this;
        ++(*this);
        return retval;
      }

      template <class OtherIteratorType>
      inline __attribute__((always_inline)) bool operator==(const OtherIteratorType& other) const noexcept {
        return lnsi_ == other.lnsi_ && vi_ == other.vi_;
      }

      inline __attribute__((always_inline)) value_type operator*() const noexcept {
        const auto& smbl_tbl = *data_->symbols_tables[lnsi_.id()];
        return make_pair(*lnsi_, smbl_tbl[*vi_]);
      }

      inline __attribute__((always_inline)) uint32_t name_id() const noexcept { return lnsi_.id(); }

      inline __attribute__((always_inline)) uint32_t value_id() const noexcept { return *vi_; }
    };

    inline __attribute__((always_inline)) auto begin() const noexcept {
      return Iterator<decltype(label_name_set_.begin()), decltype(values_begin_)>(data_, label_name_set_.begin(), values_begin_);
    }

    inline __attribute__((always_inline)) auto end() const noexcept {
      return Iterator<decltype(label_name_set_.end()), decltype(values_end_)>(data_, label_name_set_.end(), values_end_);
    }

    template <class T>
    inline __attribute__((always_inline)) bool operator==(const T& b) const noexcept {
      return std::ranges::equal(begin(), end(), b.begin(), b.end());
    }

    template <class T>
    inline __attribute__((always_inline)) bool operator<(const T& b) const noexcept {
      return std::ranges::lexicographical_compare(begin(), end(), b.begin(), b.end());
    }

    inline __attribute__((always_inline)) friend size_t hash_value(const composite_type& ls) noexcept {
      size_t res = 0;
      for (const auto& [label_name, label_value] : ls) {
        res = XXH3_64bits_withSeed(label_name.data(), label_name.size(), res) ^ XXH3_64bits_withSeed(label_value.data(), label_value.size(), res);
      }
      return res;
    }
  };

  inline __attribute__((always_inline)) composite_type composite(const data_type& data) const noexcept {
    auto lns = data.label_name_sets_table[lns_id_];

    auto [values_begin, values_end] = BareBones::StreamVByte::decoder<BareBones::StreamVByte::Codec1234>(data.symbols_ids_sequences.begin() + pos_, lns.size());

    return composite_type(&data, std::move(lns), std::move(values_begin), std::move(values_end));
  }

  inline __attribute__((always_inline)) void validate(const data_type& data) const {
    if (lns_id_ >= data.label_name_sets_table.size()) {
      throw BareBones::Exception(0x48dd6c9d357d3a7e, "LabelSets data validation error: expected LabelSets length is out of label name sets table vector range");
    }

    const auto& lns = data.label_name_sets_table[lns_id_];

    // check that streamvbyte keys are in range
    auto keys_size = BareBones::StreamVByte::keys_size(lns.size());
    if (pos_ + keys_size > data.symbols_ids_sequences.size()) {
      throw BareBones::Exception(0x22f5a82dd120e0e7, "LabelSets data validation error: expected LabelSets keys length is out of data symbols vector range");
    }

    // check that streamvbyte data is in range
    auto data_size = BareBones::StreamVByte::decode_data_size<BareBones::StreamVByte::Codec1234>(lns.size(), data.symbols_ids_sequences.begin() + pos_);
    if (pos_ + keys_size + data_size > data.symbols_ids_sequences.size()) {
      throw BareBones::Exception(0xd02e54dac8e1d328, "LabelSets data validation error: expected LabelSets values length is out of data symbols vector range");
    }

    // check that all symbols are in range
    auto [values_begin, _] = BareBones::StreamVByte::decoder<BareBones::StreamVByte::Codec1234>(data.symbols_ids_sequences.begin() + pos_, lns.size());
    for (auto i = lns.begin(); i != lns.end(); ++i) {
      if (*values_begin++ >= data.symbols_tables[i.id()]->size()) {
        throw BareBones::Exception(0x0f0c520ad6285f15,
                                   "LabelSets data validation error: expected LabelSets symbols length is out of data symbols vector range");
      }
    }
  }
};
}  // namespace Filaments
}  // namespace SnugComposites
}  // namespace Primitives
}  // namespace PromPP
