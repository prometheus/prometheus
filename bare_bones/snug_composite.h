#pragma once

#include <fstream>

#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Warray-bounds"
#include <parallel_hashmap/btree.h>
#include <parallel_hashmap/phmap.h>
#pragma GCC diagnostic pop

#include <scope_exit.h>

#include "bare_bones/allocator.h"
#include "bare_bones/exception.h"
#include "bare_bones/streams.h"
#include "bare_bones/vector.h"

namespace BareBones::SnugComposite {

/**
 * Serialization mode used to annotate encoded data and how to apply it on container.
 * We use Delta for save difference between two states of container. It doesn't matter
 * how first state was made (from snapshot or other delta). Snapshot may be explain
 * as delta from init (zero) state.
 */
enum class SerializationMode : char { SNAPSHOT = 1, DELTA = 2 };

template <class FilamentDataType>
concept is_shrinkable = requires(FilamentDataType& data_type) {
  { data_type.shrink_to(uint32_t()) };
};

template <class Derived>
concept has_next_item_index = requires(Derived derived) {
  { derived.next_item_index_impl() };
};

template <class Derived, class Checkpoint>
concept has_rollback = requires(Derived derived, const Checkpoint& checkpoint) {
  { derived.rollback_impl(checkpoint) };
};

template <class Derived>
concept has_after_items_load = requires(Derived derived) {
  { derived.after_items_load_impl(uint32_t()) };
};

template <class Derived, template <template <class> class> class Filament, template <class> class Vector>
class GenericDecodingTable {
  static_assert(!std::is_integral_v<typename Filament<Vector>::composite_type>, "Filament::composite_type can't be an integral type");

  template <class AnyDerived, template <template <class> class> class AnyFilament, template <class> class AnyVector>
  friend class GenericDecodingTable;

 public:
  using value_type = typename Filament<Vector>::composite_type;
  using data_type = typename Filament<Vector>::data_type;  // FIXME make it private

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

    const GenericDecodingTable* decoding_table;
    inline __attribute__((always_inline)) explicit Hasher(const GenericDecodingTable* _decoding_table = nullptr) noexcept : decoding_table(_decoding_table) {}

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

    const GenericDecodingTable* decoding_table;
    inline __attribute__((always_inline)) explicit EqualityComparator(const GenericDecodingTable* _decoding_table = nullptr) noexcept
        : decoding_table(_decoding_table) {}

    inline __attribute__((always_inline)) bool operator()(const Proxy& a, const Proxy& b) const noexcept { return a == b; }

    template <class Class>
    inline __attribute__((always_inline)) bool operator()(const Proxy& a, const Class& b) const noexcept {
      return decoding_table->items_[a].composite(decoding_table->data_) == b;
    }
  };

  struct LessComparator {
    using is_transparent = void;

    const GenericDecodingTable* decoding_table;
    inline __attribute__((always_inline)) explicit LessComparator(const GenericDecodingTable* _decoding_table = nullptr) noexcept
        : decoding_table(_decoding_table) {}

    inline __attribute__((always_inline)) bool operator()(const Proxy& a, const Proxy& b) const noexcept {
      return decoding_table->items_[a].composite(decoding_table->data_) < decoding_table->items_[b].composite(decoding_table->data_);
    }

    template <class Class>
    inline __attribute__((always_inline)) bool operator()(const Proxy& a, const Class& b) const noexcept {
      return decoding_table->items_[a].composite(decoding_table->data_) < b;
    }

    template <class Class>
      requires(!std::is_same_v<Class, Proxy>)
    inline __attribute__((always_inline)) bool operator()(const Class& a, const Proxy& b) const noexcept {
      return a < decoding_table->items_[b].composite(decoding_table->data_);
    }
  };

  template <class DecodingTable>
  class Checkpoint {
    const DecodingTable* decoding_table_;
    uint32_t next_item_index_;
    uint32_t size_;
    typename data_type::checkpoint_type data_checkpoint_;

   public:
    explicit inline __attribute__((always_inline)) Checkpoint(const DecodingTable& decoding_table, uint32_t next_item_index) noexcept
        : decoding_table_(&decoding_table),
          next_item_index_(next_item_index),
          size_(decoding_table.size()),
          data_checkpoint_(decoding_table.data().checkpoint()) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept { return size_; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t next_item_index() const noexcept { return next_item_index_; }

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
        first_to_save_i = from->next_item_index_;
        out.write(reinterpret_cast<const char*>(&from->next_item_index_), sizeof(from->next_item_index_));
      }
      const uint32_t first_item_index_in_decoding_table = decoding_table_->next_item_index() - decoding_table_->items().size();
      assert(first_to_save_i >= first_item_index_in_decoding_table);

      // write size
      uint32_t size_to_save = next_item_index_ - first_to_save_i;
      out.write(reinterpret_cast<char*>(&size_to_save), sizeof(size_to_save));
      // if there are no items to write, we finish here
      if (!size_to_save) {
        return;
      }

      // write items
      out.write(reinterpret_cast<const char*>(&decoding_table_->items()[first_to_save_i - first_item_index_in_decoding_table]),
                sizeof(Filament<Vector>) * size_to_save);

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
      size_t res = 1 + sizeof(SerializationMode);

      // index of first item in the portion
      uint32_t first_to_save_i = 0;
      if (from != nullptr) {
        first_to_save_i = from->next_item_index_;
        res += sizeof(uint32_t);
      }

      // size
      const uint32_t size_to_save = next_item_index_ - first_to_save_i;
      res += sizeof(uint32_t);

      // if there are no items to write, we finish here
      if (!size_to_save)
        return res;

      // items
      res += sizeof(Filament<Vector>) * size_to_save;

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

      [[nodiscard]] bool empty() const noexcept { return from_->size() >= to_->size(); }

      template <OutputStream S>
      friend S& operator<<(S& out, Delta dt) {
        dt.to_->save(out, dt.from_);
        return out;
      }

      [[nodiscard]] size_t save_size() const noexcept { return to_->save_size(from_); }
    };

    Delta operator-(const Checkpoint& from) const noexcept { return Delta(from, *this); }
  };

  static constexpr bool kIsReadOnly = std::same_as<Vector<uint8_t>, SharedSpan<uint8_t>>;

  [[nodiscard]] PROMPP_ALWAYS_INLINE Hasher hasher() const noexcept { return Hasher(this); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE EqualityComparator equality_comparator() const noexcept { return EqualityComparator(this); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE LessComparator less_comparator() const noexcept { return LessComparator(this); }

  data_type data_;
  Vector<Filament<Vector>> items_;

 public:
  using checkpoint_type = Checkpoint<GenericDecodingTable>;
  using delta_type = typename checkpoint_type::Delta;

  GenericDecodingTable() noexcept = default;
  GenericDecodingTable(const GenericDecodingTable& other) = delete;

  template <class AnotherDerived, template <template <class> class> class AnotherFilament, template <class> class AnotherVector>
    requires kIsReadOnly
  explicit GenericDecodingTable(const GenericDecodingTable<AnotherDerived, AnotherFilament, AnotherVector>& other) : data_(other.data_), items_(other.items_) {}

  GenericDecodingTable(GenericDecodingTable&& other) noexcept = delete;
  GenericDecodingTable& operator=(const GenericDecodingTable& other) = delete;
  GenericDecodingTable& operator=(GenericDecodingTable&& other) noexcept = delete;

  inline __attribute__((always_inline)) value_type operator[](uint32_t id) const noexcept { return items_[id].composite(data_); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return items_.size(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return data_.allocated_memory() + items_.allocated_memory(); }

  inline __attribute__((always_inline)) const data_type& data() const noexcept { return data_; }

  inline __attribute__((always_inline)) const auto& items() const noexcept { return items_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t next_item_index() const noexcept {
    if constexpr (has_next_item_index<Derived>) {
      return static_cast<const Derived*>(this)->next_item_index_impl();
    } else {
      return items_.size();
    }
  }

  inline __attribute__((always_inline)) auto checkpoint() const noexcept { return checkpoint_type(*this, items_.size()); }

  PROMPP_ALWAYS_INLINE void rollback(const checkpoint_type& s) noexcept
    requires(!kIsReadOnly)
  {
    if constexpr (has_rollback<Derived, checkpoint_type>) {
      static_cast<Derived*>(this)->rollback_impl(s);
    }

    if constexpr (!kIsReadOnly) {
      assert(s.size() <= items_.size());
      items_.resize(s.size());
      data_.rollback(s.data_checkpoint());
    }
  }

  template <InputStream S>
  void load(S& in) {
    // read version
    const uint8_t version = in.get();

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
    const auto mode = static_cast<SerializationMode>(in.get());

    // read index of first item in the portion, if we are reading wal
    uint32_t first_to_load_i = 0;
    if (mode == SerializationMode::DELTA) {
      in.read(reinterpret_cast<char*>(&first_to_load_i), sizeof(first_to_load_i));
    }
    if (first_to_load_i != next_item_index()) {
      if (mode == SerializationMode::SNAPSHOT) {
        throw BareBones::Exception(0x7bcd6011e39bbabc, "Attempt to load snapshot into non-empty DecodingTable");
      } else if (first_to_load_i < items_.size()) {
        throw BareBones::Exception(0x3387739a7b4f574a, "Attempt to load segment over existing DecodingTable data");
      } else {
        throw BareBones::Exception(0x4ece66e098927bc6,
                                   "Attempt to load incomplete data from segment, DecodingTable data vector length (%u) is less than segment size(%d)",
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
    const auto original_size = items_.size();
    auto sg2 = std::experimental::scope_fail([&]() { items_.resize(original_size); });
    items_.resize(items_.size() + size_to_load);
    in.read(reinterpret_cast<char*>(&items_[original_size]), sizeof(Filament<Vector>) * size_to_load);

    // read data
    auto data_checkpoint = data_.checkpoint();
    auto sg3 = std::experimental::scope_fail([&]() { data_.rollback(data_checkpoint); });
    data_.load(in);

    // validate each item (check ranges, etc)
    for (auto i = original_size; i != items_.size(); ++i) {
      items_[i].validate(data_);
    }

    // post processing
    if constexpr (has_after_items_load<Derived>) {
      static_assert(noexcept(static_cast<Derived*>(this)->after_items_load_impl(original_size)));
      static_cast<Derived*>(this)->after_items_load_impl(original_size);
    }
  }

  template <OutputStream S>
  friend S& operator<<(S& out, GenericDecodingTable& decoding_table) {
    out << decoding_table.checkpoint();
    return out;
  }

  template <InputStream S>
  friend S& operator>>(S& in, GenericDecodingTable& decoding_table) {
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
    const GenericDecodingTable* decoding_table_;
    InnerIteratorType i_;

   public:
    using iterator_category = std::input_iterator_tag;
    using value_type = typename Filament<Vector>::composite_type;
    using difference_type = std::ptrdiff_t;

    inline __attribute__((always_inline)) explicit ItemIterator(const GenericDecodingTable* decoding_table = nullptr,
                                                                InnerIteratorType i = InnerIteratorType()) noexcept
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
    const GenericDecodingTable* decoding_table_;
    InnerIteratorType i_;

   public:
    using iterator_category = std::input_iterator_tag;
    using value_type = typename Filament<Vector>::composite_type;
    using difference_type = std::ptrdiff_t;

    inline __attribute__((always_inline)) explicit ItemIDIterator(const GenericDecodingTable* decoding_table = nullptr,
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

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t id() const noexcept { return *i_; }
  };

  template <class InnerIteratorType, class InnerIteratorSentinelType>
  class Resolver {
    using inner_iterator_type = InnerIteratorType;
    using inner_iterator_sentinel_type = InnerIteratorSentinelType;

    const GenericDecodingTable* decoding_table_;
    inner_iterator_type begin_;
    inner_iterator_sentinel_type end_;

   public:
    using value_type = typename Filament<Vector>::composite_type;

    inline __attribute__((always_inline)) explicit Resolver(const GenericDecodingTable* decoding_table = nullptr,
                                                            inner_iterator_type begin = inner_iterator_type(),
                                                            inner_iterator_sentinel_type end = inner_iterator_sentinel_type()) noexcept
        : decoding_table_(decoding_table), begin_(begin), end_(end) {}

    inline __attribute__((always_inline)) auto begin() const noexcept { return ItemIDIterator<inner_iterator_type>(decoding_table_, begin_); }
    inline __attribute__((always_inline)) auto end() const noexcept { return IteratorSentinel<inner_iterator_sentinel_type>(end_); }
    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept { return end_ - begin_; }
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

  inline __attribute__((always_inline)) auto remainder_size() const noexcept { return this->data_.remainder_size(); }
};

template <template <template <class> class> class Filament, template <class> class Vector>
class DecodingTable final : public GenericDecodingTable<DecodingTable<Filament, Vector>, Filament, Vector> {
 public:
  using Base = GenericDecodingTable<DecodingTable, Filament, Vector>;

  using Base::Base;
};

template <template <template <class> class> class Filament, template <class> class Vector>
  requires is_shrinkable<typename Filament<Vector>::data_type>
class ShrinkableEncodingBimap final : private GenericDecodingTable<ShrinkableEncodingBimap<Filament, Vector>, Filament, Vector> {
 public:
  using Base = GenericDecodingTable<ShrinkableEncodingBimap, Filament, Vector>;
  using checkpoint_type = typename Base::template Checkpoint<ShrinkableEncodingBimap>;
  using delta_type = typename checkpoint_type::Delta;
  using value_type = typename Base::value_type;

  using Base::data;
  using Base::items;
  using Base::load;
  using Base::next_item_index;
  using Base::size;

  friend class GenericDecodingTable<ShrinkableEncodingBimap, Filament, Vector>;

  template <class Class>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const Class& c) noexcept {
    return *set_.lazy_emplace(c, [&](const auto& ctor) {
      ctor(Base::items_.size());
      Base::items_.emplace_back(Base::data_, c);
    }) + shift_;
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const Class& c, size_t hashval) noexcept {
    return *set_.lazy_emplace_with_hash(c, phmap::phmap_mix<sizeof(size_t)>()(hashval), [&](const auto& ctor) {
      ctor(Base::items_.size());
      Base::items_.emplace_back(Base::data_, c);
    }) + shift_;
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE std::optional<uint32_t> find(const Class& c) const noexcept {
    if (auto i = set_.find(c); i != set_.end()) {
      return *i + shift_;
    }
    return {};
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE std::optional<uint32_t> find(const Class& c, size_t hashval) const noexcept {
    if (auto i = set_.find(c, phmap::phmap_mix<sizeof(size_t)>()(hashval)); i != set_.end()) {
      return *i + shift_;
    }
    return {};
  }

  PROMPP_ALWAYS_INLINE auto checkpoint() const noexcept { return checkpoint_type(*this, next_item_index_impl()); }

  void shrink_to_checkpoint_size(const checkpoint_type& checkpoint) {
    if (checkpoint.next_item_index() != next_item_index_impl()) {
      throw Exception(0x1bf0dbff9fe3d955, "Invalid checkpoint to shrink: checkpoint next_item_index [%u], next_item_index [%u]", checkpoint.next_item_index(),
                      next_item_index_impl());
    }

    shift_ += Base::items_.size();
    Base::items_.resize(0);
    Base::items_.shrink_to_fit();
    Base::data_.shrink_to(0);
    set_.clear();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return Base::allocated_memory() + set_allocated_memory_; }

  PROMPP_ALWAYS_INLINE value_type operator[](uint32_t id) const noexcept {
    assert(id >= shift_);
    return Base::operator[](id - shift_);
  }

  template <InputStream S>
  friend S& operator>>(S& in, ShrinkableEncodingBimap& shrinkable_encoding_bimap) {
    shrinkable_encoding_bimap.load(in);
    return in;
  }

 private:
  uint32_t set_allocated_memory_{};
  phmap::flat_hash_set<typename Base::Proxy, typename Base::Hasher, typename Base::EqualityComparator, Allocator<typename Base::Proxy, uint32_t>> set_{
      {},
      0,
      Base::hasher(),
      Base::equality_comparator(),
      Allocator<typename Base::Proxy, uint32_t>{set_allocated_memory_}};
  uint32_t shift_{0};

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t next_item_index_impl() const noexcept { return shift_ + Base::items_.size(); }

  PROMPP_ALWAYS_INLINE void after_items_load_impl(uint32_t first_loaded_id) noexcept {
    set_.reserve(Base::items_.size());
    for (auto id = first_loaded_id; id != Base::items_.size(); ++id) {
      set_.emplace(typename Base::Proxy(id));
    }
  }
};

template <template <template <class> class> class Filament, template <class> class Vector>
class EncodingBimap : public GenericDecodingTable<EncodingBimap<Filament, Vector>, Filament, Vector> {
  using Base = GenericDecodingTable<EncodingBimap, Filament, Vector>;

  friend class GenericDecodingTable<EncodingBimap, Filament, Vector>;

  phmap::flat_hash_set<typename Base::Proxy, typename Base::Hasher, typename Base::EqualityComparator> set_{{}, 0, Base::hasher(), Base::equality_comparator()};

  PROMPP_ALWAYS_INLINE void after_items_load_impl(uint32_t first_loaded_id) noexcept {
    set_.reserve(Base::items_.size());
    for (auto id = first_loaded_id; id != Base::items_.size(); ++id) {
      set_.emplace(typename Base::Proxy(id));
    }
  }

 public:
  EncodingBimap() noexcept = default;
  EncodingBimap(const EncodingBimap& other) = delete;
  EncodingBimap(EncodingBimap&&) noexcept = delete;
  EncodingBimap& operator=(const EncodingBimap&) = delete;
  EncodingBimap& operator=(EncodingBimap&&) noexcept = delete;

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

  PROMPP_ALWAYS_INLINE void rollback_impl(const typename Base::checkpoint_type& s) noexcept
    requires(!Base::kIsReadOnly)
  {
    assert(s.size() <= Base::items_.size());

    for (uint32_t i = s.size(); i != Base::items_.size(); ++i) {
      set_.erase(typename Base::Proxy(i));
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return Base::allocated_memory() + set_.capacity() * sizeof(typename Base::Proxy);
  }
};

template <template <template <class> class> class Filament, template <class> class Vector>
class ParallelEncodingBimap : public GenericDecodingTable<ParallelEncodingBimap<Filament, Vector>, Filament, Vector> {
  using Base = GenericDecodingTable<ParallelEncodingBimap, Filament, Vector>;

  friend class GenericDecodingTable<ParallelEncodingBimap, Filament, Vector>;

  phmap::parallel_flat_hash_set<typename Base::Proxy, typename Base::Hasher, typename Base::EqualityComparator> set_;

  PROMPP_ALWAYS_INLINE void after_items_load_impl(uint32_t first_loaded_id) noexcept {
    set_.reserve(Base::items_.size());
    for (auto id = first_loaded_id; id != Base::items_.size(); ++id) {
      set_.emplace(typename Base::Proxy(id));
    }
  }

 public:
  inline __attribute__((always_inline)) ParallelEncodingBimap() noexcept : set_({}, 0, Base::hasher(), Base::equality_comparator()) {}

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

  inline __attribute__((always_inline)) void rollback_impl(const typename Base::checkpoint_type& s) noexcept
    requires(!Base::kIsReadOnly)
  {
    assert(s.size() <= Base::items_.size());

    for (uint32_t i = s.size(); i != Base::items_.size(); ++i) {
      set_.erase(typename Base::Proxy(i));
    }
  }

  // TODO wrap everything with read/write mutex, and test it!
};

template <template <template <class> class> class Filament, template <class> class Vector>
class OrderedEncodingBimap : public GenericDecodingTable<OrderedEncodingBimap<Filament, Vector>, Filament, Vector> {
  using Base = GenericDecodingTable<OrderedEncodingBimap, Filament, Vector>;

  friend class GenericDecodingTable<OrderedEncodingBimap, Filament, Vector>;

  using Set = phmap::btree_set<typename Base::Proxy, typename Base::LessComparator>;
  Set set_;

 protected:
  PROMPP_ALWAYS_INLINE void after_items_load_impl(uint32_t first_loaded_id) noexcept {
    for (auto id = first_loaded_id; id != Base::items_.size(); ++id) {
      set_.emplace(typename Base::Proxy(id));
    }
  }

 public:
  inline __attribute__((always_inline)) OrderedEncodingBimap() noexcept : set_({}, Base::less_comparator()) {}

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
  inline __attribute__((always_inline)) uint32_t find_or_emplace(const Class& c, [[maybe_unused]] size_t hashval) noexcept {
    return find_or_emplace(c);
  }

  template <class Class>
  inline __attribute__((always_inline)) std::optional<uint32_t> find(const Class& c) const noexcept {
    if (auto i = set_.find(c); i != set_.end()) {
      return *i;
    }
    return {};
  }

  template <class Class>
  inline __attribute__((always_inline)) std::optional<uint32_t> find(const Class& c, [[maybe_unused]] size_t hashval) const noexcept {
    if (auto i = set_.find(c); i != set_.end()) {
      return *i;
    }
    return {};
  }

  PROMPP_ALWAYS_INLINE void rollback_impl(const typename Base::checkpoint_type& s) noexcept
    requires(!Base::kIsReadOnly)
  {
    assert(s.size() <= Base::items_.size());

    for (uint32_t i = s.size(); i != Base::items_.size(); ++i) {
      set_.erase(typename Base::Proxy(i));
    }
  }

  inline __attribute__((always_inline)) auto begin() const noexcept {
    return typename Base::template ItemIDIterator<typename Set::const_iterator>(this, set_.begin());
  }
  inline __attribute__((always_inline)) auto end() const noexcept { return typename Base::template IteratorSentinel<typename Set::const_iterator>(set_.end()); }

  inline __attribute__((always_inline)) auto unordered_begin() const noexcept { return Base::begin(); }
  inline __attribute__((always_inline)) auto unordered_end() const noexcept { return Base::end(); }
};

template <template <template <class> class> class Filament, template <class> class Vector>
class OrderedDecodingTable : public GenericDecodingTable<OrderedDecodingTable<Filament, Vector>, Filament, Vector> {
  using Base = GenericDecodingTable<OrderedDecodingTable, Filament, Vector>;

 public:
  using Base::Base;
  OrderedDecodingTable(const OrderedDecodingTable&) = delete;
  OrderedDecodingTable(OrderedDecodingTable&&) = delete;
  OrderedDecodingTable& operator=(const OrderedDecodingTable&) = delete;
  OrderedDecodingTable& operator=(OrderedDecodingTable&&) = delete;

  auto back() const noexcept { return Base::items_.back().composite(Base::data_); }

  template <class Class>
  inline __attribute__((always_inline)) uint32_t emplace_back(const Class& c) {
    const uint32_t id = Base::items_.size();

    if (id != 0 && c < back()) {
      throw BareBones::Exception(0xf677f03159e75ee7, "Broken order of OrderedDecodingTable item emplacement");
    }

    Base::items_.emplace_back(Base::data_, c);

    return id;
  }
};

template <template <template <class> class> class Filament, template <class> class Vector>
class EncodingBimapWithOrderedAccess : public GenericDecodingTable<EncodingBimapWithOrderedAccess<Filament, Vector>, Filament, Vector> {
  using Base = GenericDecodingTable<EncodingBimapWithOrderedAccess, Filament, Vector>;

  friend class GenericDecodingTable<EncodingBimapWithOrderedAccess, Filament, Vector>;

  using OrderedSet = phmap::btree_set<typename Base::Proxy, typename Base::LessComparator>;
  OrderedSet ordered_set_;

  using Set = phmap::flat_hash_set<typename Base::Proxy, typename Base::Hasher, typename Base::EqualityComparator>;
  Set set_;

  PROMPP_ALWAYS_INLINE void after_items_load_impl(uint32_t first_loaded_id) noexcept {
    set_.reserve(Base::items_.size());
    for (auto id = first_loaded_id; id != Base::items_.size(); ++id) {
      set_.emplace(typename Base::Proxy(id));
      ordered_set_.emplace(typename Base::Proxy(id));
    }
  }

 public:
  inline __attribute__((always_inline)) EncodingBimapWithOrderedAccess() noexcept
      : ordered_set_({}, Base::less_comparator()), set_({}, 0, Base::hasher(), Base::equality_comparator()) {}

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

  PROMPP_ALWAYS_INLINE void rollback_impl(const typename Base::checkpoint_type& s) noexcept
    requires(!Base::kIsReadOnly)
  {
    assert(s.size() <= Base::items_.size());

    for (uint32_t i = s.size(); i != Base::items_.size(); ++i) {
      ordered_set_.erase(typename Base::Proxy(i));
      set_.erase(typename Base::Proxy(i));
    }
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

}  // namespace BareBones::SnugComposite
