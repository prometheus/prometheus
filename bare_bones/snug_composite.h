#pragma once

#include <fstream>
#include <string_view>

#include <parallel_hashmap/btree.h>
#include <parallel_hashmap/phmap.h>

#include <scope_exit.h>

#include "bare_bones/exception.h"
#include "bare_bones/streams.h"
#include "bare_bones/vector.h"

namespace BareBones {
namespace SnugComposite {
/**
 * Serialization mode used to annotate encoded data and how to apply it on container.
 * We use Delta for save difference between two states of container. It doesn't matter
 * how first state was made (from snapshot or other delta). Snapshot may be explain
 * as delta from init (zero) state.
 */
enum class SerializationMode : char { SNAPSHOT = 1, DELTA = 2 };

template <class Filament>
class DecodingTable {
  static_assert(!std::is_integral<typename Filament::composite_type>::value, "Filament::composite_type can't be an integral type");

 public:
  using value_type = typename Filament::composite_type;
  using data_type = typename Filament::data_type;  // FIXME make it private

 protected:
  class Proxy {
    uint32_t id_;

   public:
    // NOLINTNEXTLINE(google-explicit-constructor)
    inline __attribute__((always_inline)) Proxy(uint32_t id) noexcept : id_(id) {}

    // NOLINTNEXTLINE(google-explicit-constructor)
    inline __attribute__((always_inline)) operator uint32_t() const noexcept { return id_; }

    inline __attribute__((always_inline)) bool operator==(const Proxy& o) noexcept { return id_ == o.id_; }
  };

  struct Hasher {
    using is_transparent = void;

    const DecodingTable* decoding_table;
    inline __attribute__((always_inline)) explicit Hasher(const DecodingTable* _decoding_table = nullptr) noexcept : decoding_table(_decoding_table) {}

    template <class Class>
    inline __attribute__((always_inline)) size_t operator()(const Class& c) const noexcept {
      return phmap::Hash<Class>()(c);
    }

    inline __attribute__((always_inline)) size_t operator()(const Proxy& p) const noexcept {
      auto composite = decoding_table->items_[p].composite(decoding_table->data_);
      return phmap::Hash<decltype(composite)>()(composite);
    }
  };

  struct EqualityComparator {
    using is_transparent = void;

    const DecodingTable* decoding_table;
    inline __attribute__((always_inline)) explicit EqualityComparator(const DecodingTable* _decoding_table = nullptr) noexcept
        : decoding_table(_decoding_table) {}

    inline __attribute__((always_inline)) bool operator()(const Proxy& a, const Proxy& b) const noexcept { return a == b; }

    template <class Class>
    inline __attribute__((always_inline)) bool operator()(const Proxy& a, const Class& b) const noexcept {
      return decoding_table->items_[a].composite(decoding_table->data_) == b;
    }
  };

  struct LessComparator {
    using is_transparent = void;

    const DecodingTable* decoding_table;
    inline __attribute__((always_inline)) explicit LessComparator(const DecodingTable* _decoding_table = nullptr) noexcept : decoding_table(_decoding_table) {}

    inline __attribute__((always_inline)) bool operator()(const Proxy& a, const Proxy& b) const noexcept {
      return decoding_table->items_[a].composite(decoding_table->data_) < decoding_table->items_[b].composite(decoding_table->data_);
    }

    template <class Class>
    inline __attribute__((always_inline)) bool operator()(const Proxy& a, const Class& b) const noexcept {
      return decoding_table->items_[a].composite(decoding_table->data_) < b;
    }

    template <class Class>
      requires(!std::is_same<Class, Proxy>::value)
    inline __attribute__((always_inline)) bool operator()(const Class& a, const Proxy& b) const noexcept {
      return a < decoding_table->items_[b].composite(decoding_table->data_);
    }
  };

  class Checkpoint {
    DecodingTable<Filament> const* decoding_table_;
    uint32_t size_;
    typename data_type::checkpoint_type data_checkpoint_;

   public:
    inline __attribute__((always_inline)) Checkpoint(DecodingTable<Filament> const& decoding_table) noexcept
        : decoding_table_(&decoding_table), size_(decoding_table.size()), data_checkpoint_(decoding_table.data().checkpoint()) {}

    size_t size() const noexcept { return size_; }

    const typename data_type::checkpoint_type& data_checkpoint() const noexcept { return data_checkpoint_; }

    template <OutputStream S>
    void save(S& out, const Checkpoint* from = nullptr) const {
      auto original_exceptions = out.exceptions();
      auto sg1 = std::experimental::scope_exit([&]() { out.exceptions(original_exceptions); });
      out.exceptions(std::ifstream::failbit | std::ifstream::badbit);

      // write version
      out.put(1);

      // write mode
      SerializationMode mode = (from != nullptr) ? SerializationMode::DELTA : SerializationMode::SNAPSHOT;
      out.put(static_cast<char>(mode));

      // write index of first item in the portion
      uint32_t first_to_save_i = 0;
      if (from != nullptr) {
        first_to_save_i = from->size_;
        out.write(reinterpret_cast<char*>(&first_to_save_i), sizeof(first_to_save_i));
      }

      // write size
      uint32_t size_to_save = size_ - first_to_save_i;
      out.write(reinterpret_cast<char*>(&size_to_save), sizeof(size_to_save));

      // if there are no items to write, we finish here
      if (!size_to_save) {
        return;
      }

      // write items
      out.write(reinterpret_cast<const char*>(&decoding_table_->items()[first_to_save_i]), sizeof(Filament) * size_to_save);

      // write data
      if (from != nullptr) {
        data_checkpoint_.save(out, decoding_table_->data(), &from->data_checkpoint_);
      } else {
        data_checkpoint_.save(out, decoding_table_->data());
      }
    }

    template <OutputStream S>
    friend S& operator<<(S& out, const Checkpoint& cp) {
      cp.save(out);
      return out;
    }

    size_t save_size(const Checkpoint* from = nullptr) const noexcept {
      // version is written and read by methods put() and get() and they write and read 1 byte
      SerializationMode mode = (from != nullptr) ? SerializationMode::DELTA : SerializationMode::SNAPSHOT;
      size_t res = 1 + sizeof(mode);

      // index of first item in the portion
      uint32_t first_to_save_i = 0;
      if (from != nullptr) {
        first_to_save_i = from->size_;
        res += sizeof(uint32_t);
      }

      // size
      uint32_t size_to_save = size_ - first_to_save_i;
      res += sizeof(uint32_t);

      // if there are no items to write, we finish here
      if (!size_to_save)
        return res;

      // items
      res += sizeof(Filament) * size_to_save;

      // data
      if (from != nullptr) {
        res += data_checkpoint_.save_size(decoding_table_->data(), &from->data_checkpoint_);
      } else {
        res += data_checkpoint_.save_size(decoding_table_->data());
      }

      return res;
    }

    /**
     * ATTENTION! This class persists only pointers to checkpoint. It's a user resposability
     * to prevent using delta out of checkpoints scope!
     */
    class Delta {
      Checkpoint const* from_;
      Checkpoint const* to_;

     public:
      inline __attribute__((always_inline)) Delta(Checkpoint const& from, Checkpoint const& to) noexcept : from_(&from), to_(&to) {}

      bool empty() const noexcept { return from_->size() >= to_->size(); }

      template <OutputStream S>
      friend S& operator<<(S& out, Delta dt) {
        dt.to_->save(out, dt.from_);
        return out;
      }

      size_t save_size() const noexcept { return to_->save_size(from_); }
    };

    Delta operator-(const Checkpoint& from) const noexcept { return Delta(from, *this); }
  };

  Hasher hasher_;
  EqualityComparator equality_comparator_;
  LessComparator less_comparator_;

  data_type data_;
  Vector<Filament> items_;

  virtual void after_items_load(uint32_t first_loaded_id) noexcept {}

 public:
  using checkpoint_type = Checkpoint;
  using delta_type = typename Checkpoint::Delta;

  inline __attribute__((always_inline)) DecodingTable() noexcept : hasher_(this), equality_comparator_(this), less_comparator_(this) {}

  DecodingTable(const DecodingTable&) = delete;
  DecodingTable(DecodingTable&&) = delete;
  DecodingTable& operator=(const DecodingTable&) = delete;
  DecodingTable& operator=(DecodingTable&&) = delete;

  inline __attribute__((always_inline)) value_type operator[](uint32_t id) const noexcept { return items_[id].composite(data_); }

  inline __attribute__((always_inline)) uint32_t size() const noexcept { return items_.size(); }

  inline __attribute__((always_inline)) const data_type& data() const noexcept { return data_; }

  inline __attribute__((always_inline)) const auto& items() const noexcept { return items_; }

  inline __attribute__((always_inline)) auto checkpoint() const noexcept { return Checkpoint(*this); }

  inline __attribute__((always_inline)) virtual void rollback(const checkpoint_type& s) noexcept {
    assert(s.size() <= items_.size());
    items_.resize(s.size());
    data_.rollback(s.data_checkpoint());
  }

  template <InputStream S>
  void load(S& in) {
    // read version
    uint8_t version = in.get();

    // return successfully, if stream is empty
    if (in.eof())
      return;

    // check version
    if (version != 1) {
      throw BareBones::Exception(0x343b7bcc6814f2d5, "Invalid EncodingSequence version %d got from input stream, only version 1 is supported", version);
    }

    auto original_exceptions = in.exceptions();
    auto sg1 = std::experimental::scope_exit([&]() { in.exceptions(original_exceptions); });
    in.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit);

    // read mode
    SerializationMode mode = static_cast<SerializationMode>(in.get());

    // read index of first item in the portion, if we are reading wal
    uint32_t first_to_load_i = 0;
    if (mode == SerializationMode::DELTA) {
      in.read(reinterpret_cast<char*>(&first_to_load_i), sizeof(first_to_load_i));
    }
    if (first_to_load_i != items_.size()) {
      if (mode == SerializationMode::SNAPSHOT) {
        throw BareBones::Exception(0x7bcd6011e39bbabc, "Attempt to load snapshot into non-empty DecodingTable");
      } else if (first_to_load_i < items_.size()) {
        throw BareBones::Exception(0x3387739a7b4f574a, "Attempt to load segment over existing DecodingTable data");
      } else {
        throw BareBones::Exception(0x4ece66e098927bc6,
                                   "Attempt to load incomplete data from segment, DecodingTable data vector length (%zd) is less than segment size(%d)",
                                   items_.size(), first_to_load_i);
      }
    }

    // read size
    uint32_t size_to_load;
    in.read(reinterpret_cast<char*>(&size_to_load), sizeof(size_to_load));

    // read is completed, if there are no items
    if (!size_to_load) {
      return;
    }

    // read items
    auto original_size = items_.size();
    auto sg2 = std::experimental::scope_fail([&]() { items_.resize(original_size); });
    items_.resize(items_.size() + size_to_load);
    in.read(reinterpret_cast<char*>(&items_[first_to_load_i]), sizeof(Filament) * size_to_load);

    // read data
    auto data_checkpoint = data_.checkpoint();
    auto sg3 = std::experimental::scope_fail([&]() { data_.rollback(data_checkpoint); });
    data_.load(in);

    // validate each item (check ranges, etc)
    for (auto i = first_to_load_i; i != items_.size(); ++i) {
      items_[i].validate(data_);
    }

    // post processing
    static_assert(noexcept(after_items_load(first_to_load_i)));
    after_items_load(first_to_load_i);
  }

  template <OutputStream S>
  friend S& operator<<(S& out, DecodingTable& decoding_table) {
    out << decoding_table.checkpoint();
    return out;
  }

  template <InputStream S>
  friend S& operator>>(S& in, DecodingTable& decoding_table) {
    decoding_table.load(in);
    return in;
  }

  template <class InnerIteratorType>
  class IteratorSentinel {
    InnerIteratorType i_;

   public:
    inline __attribute__((always_inline)) explicit IteratorSentinel(InnerIteratorType i = InnerIteratorType()) noexcept : i_(i) {}

    inline __attribute__((always_inline)) const InnerIteratorType& inner_iterator() const noexcept { return i_; }
  };

  template <class InnerIteratorType>
  class ItemIterator {
    const DecodingTable* decoding_table_;
    InnerIteratorType i_;

   public:
    using iterator_category = std::input_iterator_tag;
    using value_type = typename Filament::composite_type;
    using difference_type = std::ptrdiff_t;

    inline
        __attribute__((always_inline)) explicit ItemIterator(const DecodingTable* decoding_table = nullptr, InnerIteratorType i = InnerIteratorType()) noexcept
        : decoding_table_(decoding_table), i_(i) {}

    inline __attribute__((always_inline)) ItemIterator& operator++() noexcept {
      ++i_;
      return *this;
    }

    inline __attribute__((always_inline)) ItemIterator operator++(int) noexcept {
      ItemIterator retval = *this;
      ++(*this);
      return retval;
    }

    inline __attribute__((always_inline)) bool operator==(const ItemIterator& other) const noexcept { return i_ == other.i_; }

    template <class InnerIteratorSentinelType>
    inline __attribute__((always_inline)) bool operator==(const IteratorSentinel<InnerIteratorSentinelType>& other) const noexcept {
      return i_ == other.inner_iterator();
    }

    inline __attribute__((always_inline)) value_type operator*() const noexcept { return i_->composite(decoding_table_->data()); }
  };

  template <class InnerIteratorType>
  class ItemIDIterator {
    const DecodingTable* decoding_table_;
    InnerIteratorType i_;

   public:
    using iterator_category = std::input_iterator_tag;
    using value_type = typename Filament::composite_type;
    using difference_type = std::ptrdiff_t;

    inline __attribute__((always_inline)) explicit ItemIDIterator(const DecodingTable* decoding_table = nullptr,
                                                                  InnerIteratorType i = InnerIteratorType()) noexcept
        : decoding_table_(decoding_table), i_(i) {}
    inline __attribute__((always_inline)) ItemIDIterator& operator++() {
      ++i_;
      return *this;
    }

    inline __attribute__((always_inline)) ItemIDIterator operator++(int) noexcept {
      ItemIDIterator retval = *this;
      ++(*this);
      return retval;
    }

    inline __attribute__((always_inline)) bool operator==(const ItemIDIterator& other) const noexcept { return i_ == other.i_; }

    template <class InnerIteratorSentinelType>
    inline __attribute__((always_inline)) bool operator==(const IteratorSentinel<InnerIteratorSentinelType>& other) const noexcept {
      return i_ == other.inner_iterator();
    }

    inline __attribute__((always_inline)) value_type operator*() const noexcept { return (*decoding_table_)[*i_]; }

    inline __attribute__((always_inline)) uint32_t id() const noexcept { return *i_; }
  };

  template <class InnerIteratorType, class InnerIteratorSentinelType>
  class Resolver {
    using inner_iterator_type = InnerIteratorType;
    using inner_iterator_sentinel_type = InnerIteratorSentinelType;

    const DecodingTable* decoding_table_;
    inner_iterator_type begin_;
    inner_iterator_sentinel_type end_;

   public:
    using value_type = typename Filament::composite_type;

    inline __attribute__((always_inline)) explicit Resolver(const DecodingTable* decoding_table = nullptr,
                                                            inner_iterator_type begin = inner_iterator_type(),
                                                            inner_iterator_sentinel_type end = inner_iterator_sentinel_type()) noexcept
        : decoding_table_(decoding_table), begin_(begin), end_(end) {}

    inline __attribute__((always_inline)) auto begin() const noexcept { return ItemIDIterator<inner_iterator_type>(decoding_table_, begin_); }
    inline __attribute__((always_inline)) auto end() const noexcept { return IteratorSentinel<inner_iterator_sentinel_type>(end_); }
    inline __attribute__((always_inline)) size_t size() const noexcept { return end_ - begin_; }
  };

  template <class IteratorType, class IteratorSentinelType>
  inline __attribute__((always_inline)) Resolver<IteratorType, IteratorSentinelType> resolve(IteratorType begin, IteratorSentinelType end) const noexcept {
    return Resolver<IteratorType, IteratorSentinelType>(this, begin, end);
  }

  template <class R>
  inline __attribute__((always_inline)) auto resolve(const R& range) const noexcept {
    return resolve(range.begin(), range.end());
  }

  inline __attribute__((always_inline)) auto begin() const noexcept { return ItemIterator<decltype(items_.begin())>(this, items_.begin()); }
  inline __attribute__((always_inline)) auto end() const noexcept { return IteratorSentinel<decltype(items_.end())>(items_.end()); }

  virtual ~DecodingTable() = default;
};

template <class Filament>
class EncodingBimap : public DecodingTable<Filament> {
  using Base = DecodingTable<Filament>;

  phmap::flat_hash_set<typename Base::Proxy, typename Base::Hasher, typename Base::EqualityComparator> set_;

  inline __attribute__((always_inline)) virtual void after_items_load(uint32_t first_loaded_id) noexcept {
    set_.reserve(Base::items_.size());
    for (auto id = first_loaded_id; id != Base::items_.size(); ++id) {
      set_.emplace(typename Base::Proxy(id));
    }
  }

 public:
  inline __attribute__((always_inline)) EncodingBimap() noexcept : set_({}, 0, Base::hasher_, Base::equality_comparator_) {}

  EncodingBimap(const EncodingBimap&) = delete;
  EncodingBimap(EncodingBimap&&) = delete;
  EncodingBimap& operator=(const EncodingBimap&) = delete;
  EncodingBimap& operator=(EncodingBimap&&) = delete;

  template <class Class>
  inline __attribute__((always_inline)) uint32_t find_or_emplace(const Class& c) noexcept {
    return *set_.lazy_emplace(c, [&](const auto& ctor) {
      uint32_t id = Base::items_.size();
      Base::items_.emplace_back(Base::data_, c);
      ctor(id);
    });
  }

  template <class Class>
  inline __attribute__((always_inline)) uint32_t find_or_emplace(const Class& c, size_t hashval) noexcept {
    return *set_.lazy_emplace_with_hash(c, phmap::phmap_mix<sizeof(size_t)>()(hashval), [&](const auto& ctor) {
      uint32_t id = Base::items_.size();
      Base::items_.emplace_back(Base::data_, c);
      ctor(id);
    });
  }

  template <class Class>
  inline __attribute__((always_inline)) std::optional<uint32_t> find(const Class& c) const noexcept {
    if (auto i = set_.find(c); i != set_.end()) {
      return *i;
    }
    return {};
  }

  template <class Class>
  inline __attribute__((always_inline)) std::optional<uint32_t> find(const Class& c, size_t hashval) const noexcept {
    if (auto i = set_.find(c, phmap::phmap_mix<sizeof(size_t)>()(hashval)); i != set_.end()) {
      return *i;
    }
    return {};
  }

  inline __attribute__((always_inline)) virtual void rollback(const typename Base::checkpoint_type& s) noexcept {
    assert(s.size() <= Base::items_.size());

    for (uint32_t i = s.size(); i != Base::items_.size(); ++i) {
      set_.erase(typename Base::Proxy(i));
    }

    Base::rollback(s);
  }
};

template <class Filament>
class ParallelEncodingBimap : public DecodingTable<Filament> {
  using Base = DecodingTable<Filament>;

  phmap::parallel_flat_hash_set<typename Base::Proxy, typename Base::Hasher, typename Base::EqualityComparator> set_;

  inline __attribute__((always_inline)) virtual void after_items_load(uint32_t first_loaded_id) noexcept {
    set_.reserve(Base::items_.size());
    for (auto id = first_loaded_id; id != Base::items_.size(); ++id) {
      set_.emplace(typename Base::Proxy(id));
    }
  }

 public:
  inline __attribute__((always_inline)) ParallelEncodingBimap() noexcept : set_({}, 0, Base::hasher_, Base::equality_comparator_) {}

  ParallelEncodingBimap(const ParallelEncodingBimap&) = delete;
  ParallelEncodingBimap(ParallelEncodingBimap&&) = delete;
  ParallelEncodingBimap& operator=(const ParallelEncodingBimap&) = delete;
  ParallelEncodingBimap& operator=(ParallelEncodingBimap&&) = delete;

  template <class Class>
  inline __attribute__((always_inline)) uint32_t find_or_emplace(const Class& c) noexcept {
    return *set_.lazy_emplace(c, [&](const auto& ctor) {
      uint32_t id = Base::items_.size();
      Base::items_.emplace_back(Base::data_, c);
      ctor(id);
    });
  }

  template <class Class>
  inline __attribute__((always_inline)) uint32_t find_or_emplace(const Class& c, size_t hashval) noexcept {
    return *set_.lazy_emplace_with_hash(c, phmap::phmap_mix<sizeof(size_t)>()(hashval), [&](const auto& ctor) {
      uint32_t id = Base::items_.size();
      Base::items_.emplace_back(Base::data_, c);
      ctor(id);
    });
  }

  template <class Class>
  inline __attribute__((always_inline)) std::optional<uint32_t> find(const Class& c) const noexcept {
    if (auto i = set_.find(c); i != set_.end()) {
      return *i;
    }
    return {};
  }

  template <class Class>
  inline __attribute__((always_inline)) std::optional<uint32_t> find(const Class& c, size_t hashval) const noexcept {
    if (auto i = set_.find(c, phmap::phmap_mix<sizeof(size_t)>()(hashval)); i != set_.end()) {
      return *i;
    }
    return {};
  }

  inline __attribute__((always_inline)) virtual void rollback(const typename Base::checkpoint_type& s) noexcept {
    assert(s.size() <= Base::items_.size());

    for (uint32_t i = s.size(); i != Base::items_.size(); ++i) {
      set_.erase(typename Base::Proxy(i));
    }

    Base::rollback(s);
  }

  // TODO wrap everything with read/write mutex, and test it!
};

template <class Filament>
class OrderedEncodingBimap : public DecodingTable<Filament> {
  using Base = DecodingTable<Filament>;

  using Set = phmap::btree_set<typename Base::Proxy, typename Base::LessComparator>;
  Set set_;

  inline __attribute__((always_inline)) virtual void after_items_load(uint32_t first_loaded_id) noexcept {
    for (auto id = first_loaded_id; id != Base::items_.size(); ++id) {
      set_.emplace(typename Base::Proxy(id));
    }
  }

 public:
  inline __attribute__((always_inline)) OrderedEncodingBimap() noexcept : set_({}, Base::less_comparator_) {}

  OrderedEncodingBimap(const OrderedEncodingBimap&) = delete;
  OrderedEncodingBimap(OrderedEncodingBimap&&) = delete;
  OrderedEncodingBimap& operator=(const OrderedEncodingBimap&) = delete;
  OrderedEncodingBimap& operator=(OrderedEncodingBimap&&) = delete;

  template <class Class>
  inline __attribute__((always_inline)) uint32_t find_or_emplace(const Class& c) noexcept {
    auto i = set_.lower_bound(c);
    if (i != set_.end() && (*this)[*i] == c) {
      return *i;
    }

    uint32_t id = Base::items_.size();
    Base::items_.emplace_back(Base::data_, c);
    set_.insert(i, typename Base::Proxy(id));

    return id;
  }

  template <class Class>
  inline __attribute__((always_inline)) std::optional<uint32_t> find(const Class& c) const noexcept {
    if (auto i = set_.find(c); i != set_.end()) {
      return *i;
    }
    return {};
  }

  inline __attribute__((always_inline)) virtual void rollback(const typename Base::checkpoint_type& s) noexcept {
    assert(s.size() <= Base::items_.size());

    for (uint32_t i = s.size(); i != Base::items_.size(); ++i) {
      set_.erase(typename Base::Proxy(i));
    }

    Base::rollback(s);
  }

  inline __attribute__((always_inline)) auto begin() const noexcept {
    return typename Base::template ItemIDIterator<typename Set::const_iterator>(this, set_.begin());
  }
  inline __attribute__((always_inline)) auto end() const noexcept { return typename Base::template IteratorSentinel<typename Set::const_iterator>(set_.end()); }

  inline __attribute__((always_inline)) auto unordered_begin() const noexcept { return Base::begin(); }
  inline __attribute__((always_inline)) auto unordered_end() const noexcept { return Base::end(); }
};

template <class Filament>
class OrderedDecodingTable : public DecodingTable<Filament> {
  using Base = DecodingTable<Filament>;

 public:
  using Base::Base;
  OrderedDecodingTable(const OrderedDecodingTable&) = delete;
  OrderedDecodingTable(OrderedDecodingTable&&) = delete;
  OrderedDecodingTable& operator=(const OrderedDecodingTable&) = delete;
  OrderedDecodingTable& operator=(OrderedDecodingTable&&) = delete;

  auto back() const noexcept { return Base::items_.back().composite(Base::data_); }

  template <class Class>
  inline __attribute__((always_inline)) uint32_t emplace_back(const Class& c) {
    uint32_t id = Base::items_.size();

    if (id != 0 && c < back()) {
      throw BareBones::Exception(0xf677f03159e75ee7, "Broken order of OrderedDecodingTable item emplacement");
    }

    Base::items_.emplace_back(Base::data_, c);

    return id;
  }
};

template <class Filament>
class EncodingBimapWithOrderedAccess : public DecodingTable<Filament> {
  using Base = DecodingTable<Filament>;

  using OrderedSet = phmap::btree_set<typename Base::Proxy, typename Base::LessComparator>;
  OrderedSet ordered_set_;

  using Set = phmap::flat_hash_set<typename Base::Proxy, typename Base::Hasher, typename Base::EqualityComparator>;
  Set set_;

  inline __attribute__((always_inline)) virtual void after_items_load(uint32_t first_loaded_id) noexcept {
    set_.reserve(Base::items_.size());
    for (auto id = first_loaded_id; id != Base::items_.size(); ++id) {
      set_.emplace(typename Base::Proxy(id));
      ordered_set_.emplace(typename Base::Proxy(id));
    }
  }

 public:
  inline __attribute__((always_inline)) EncodingBimapWithOrderedAccess() noexcept
      : ordered_set_({}, Base::less_comparator_), set_({}, 0, Base::hasher_, Base::equality_comparator_) {}

  EncodingBimapWithOrderedAccess(const EncodingBimapWithOrderedAccess&) = delete;
  EncodingBimapWithOrderedAccess(EncodingBimapWithOrderedAccess&&) = delete;
  EncodingBimapWithOrderedAccess& operator=(const EncodingBimapWithOrderedAccess&) = delete;
  EncodingBimapWithOrderedAccess& operator=(EncodingBimapWithOrderedAccess&&) = delete;

  template <class Class>
  inline __attribute__((always_inline)) uint32_t find_or_emplace(const Class& c) noexcept {
    return *set_.lazy_emplace(c, [&](const auto& ctor) {
      uint32_t id = Base::items_.size();
      Base::items_.emplace_back(Base::data_, c);
      ordered_set_.insert(typename Base::Proxy(id));
      ctor(id);
    });
  }

  template <class Class>
  inline __attribute__((always_inline)) uint32_t find_or_emplace(const Class& c, size_t hashval) noexcept {
    return *set_.lazy_emplace_with_hash(c, phmap::phmap_mix<sizeof(size_t)>()(hashval), [&](const auto& ctor) {
      uint32_t id = Base::items_.size();
      Base::items_.emplace_back(Base::data_, c);
      ordered_set_.insert(typename Base::Proxy(id));
      ctor(id);
    });
  }

  template <class Class>
  inline __attribute__((always_inline)) std::optional<uint32_t> find(const Class& c) const noexcept {
    if (auto i = set_.find(c); i != set_.end()) {
      return *i;
    }
    return {};
  }

  template <class Class>
  inline __attribute__((always_inline)) std::optional<uint32_t> find(const Class& c, size_t hashval) const noexcept {
    if (auto i = set_.find(c, phmap::phmap_mix<sizeof(size_t)>()(hashval)); i != set_.end()) {
      return *i;
    }
    return {};
  }

  inline __attribute__((always_inline)) virtual void rollback(const typename Base::checkpoint_type& s) noexcept {
    assert(s.size() <= Base::items_.size());

    for (uint32_t i = s.size(); i != Base::items_.size(); ++i) {
      ordered_set_.erase(typename Base::Proxy(i));
      set_.erase(typename Base::Proxy(i));
    }

    Base::rollback(s);
  }

  inline __attribute__((always_inline)) auto begin() const noexcept {
    return typename Base::template ItemIDIterator<typename OrderedSet::const_iterator>(this, ordered_set_.begin());
  }
  inline __attribute__((always_inline)) auto end() const noexcept {
    return typename Base::template IteratorSentinel<typename OrderedSet::const_iterator>(ordered_set_.end());
  }

  inline __attribute__((always_inline)) auto unordered_begin() const noexcept { return Base::begin(); }
  inline __attribute__((always_inline)) auto unordered_end() const noexcept { return Base::end(); }
};
}  // namespace SnugComposite
}  // namespace BareBones
