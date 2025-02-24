#pragma once

#include <algorithm>
#include <cassert>
#include <cstring>
#include <fstream>
#include <numeric>

#include <scope_exit.h>

#include "allocated_memory.h"
#include "exception.h"
#include "memory.h"
#include "preprocess.h"
#include "streams.h"
#include "type_traits.h"

namespace BareBones {

template <class Derived, class SizeType, class T>
class GenericVector {
  /**
   * Why??? Why another vector??? Why no std::vector?
   *
   * There are two main reasons for Vector:
   * - it is more compact, because of +10% growth policy (instead of x2),
   * - it uses std::realloc for allocation and reallocation.
   *
   * More information on reasons and motivation can be found in the folly/FBVector documentations.
   * Original implementation (folly/FBVector) is not used because folly is too cumbersome to install.
   */

  static_assert(IsTriviallyReallocatable<T>::value, "type parameter of this class should be trivially reallocatable");

 public:
  using iterator_category = std::contiguous_iterator_tag;
  using value_type = T;
  using iterator = T*;
  using const_iterator = const T*;

  PROMPP_ALWAYS_INLINE void reserve(SizeType size) noexcept { derived()->memory().grow_to_fit_at_least(size); }

  PROMPP_ALWAYS_INLINE void shrink_to_fit() noexcept { derived()->memory().resize_to_fit_at_least(size()); }

  PROMPP_ALWAYS_INLINE void resize(SizeType new_size) noexcept {
    reserve(new_size);

    if constexpr (!std::is_trivial_v<T>) {
      const auto current_size = size();
      auto memory = data();

      if constexpr (IsZeroInitializable<T>::value) {
        if constexpr (IsTriviallyDestructible<T>::value) {
          if (new_size > current_size) {
            zero_memory(memory + current_size, new_size - current_size);
          } else {
            zero_memory(memory + new_size, current_size - new_size);
          }
        } else {
          if (new_size > current_size) {
            zero_memory(memory + current_size, new_size - current_size);
          } else {
            memory += new_size;
            for (SizeType i = new_size; i != current_size; ++i) {
              std::destroy_at(memory++);
            }
          }
        }
      } else {
        if (new_size > current_size) {
          memory += current_size;
          for (SizeType i = current_size; i != new_size; ++i) {
            std::construct_at(memory++);
          }
        } else {
          memory += new_size;
          for (SizeType i = new_size; i != current_size; ++i) {
            std::destroy_at(memory++);
          }
        }
      }
    }

    derived()->set_size(new_size);
  }

  template <class Writer>
    requires std::is_invocable_v<Writer, iterator, SizeType>
  PROMPP_ALWAYS_INLINE void reserve_and_write(SizeType additional_size, Writer&& writer) {
    reserve(size() + additional_size);
    derived()->set_size(size() + std::forward<Writer>(writer)(end(), additional_size));
  }

  PROMPP_ALWAYS_INLINE void clear() noexcept {
    if constexpr (!std::is_trivial_v<T>) {
      auto memory = data();
      const auto current_size = size();

      if constexpr (IsTriviallyDestructible<T>::value) {
        zero_memory(memory, current_size);
      } else {
        for (SizeType i = 0; i != current_size; ++i) {
          std::destroy_at(memory++);
        }
      }
    }

    derived()->set_size(0);
  }

  PROMPP_ALWAYS_INLINE iterator erase(iterator first, iterator last) noexcept {
    assert(first >= begin());
    assert(last <= end());
    assert(last >= first);

    if (first == last) {
      return end();
    }

    if constexpr (!IsTriviallyDestructible<T>::value) {
      for (auto i = first; i != last; ++i, ++first) {
        first->~T();
      }
    }

    PRAGMA_DIAGNOSTIC(push)
    PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
    std::ranges::move(last, end(), first);
    PRAGMA_DIAGNOSTIC(pop)

    resize(size() - (last - first));
    return first;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const T* data() const noexcept { return derived()->memory(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE T* data() noexcept { return derived()->memory(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool empty() const noexcept { return size() == 0; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE SizeType capacity() const noexcept { return derived()->memory().size(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return mem::allocated_memory(derived()->memory()) +
           std::accumulate(begin(), end(), 0, [](size_t memory, const auto& item) PROMPP_LAMBDA_INLINE { return memory + mem::allocated_memory(item); });
  }

  template <class Item>
  PROMPP_ALWAYS_INLINE void push_back(Item&& item) noexcept {
    auto pos = size();
    resize(pos + 1);
    std::construct_at(data() + pos, std::forward<Item>(item));
  }

  template <class Item>
  PROMPP_ALWAYS_INLINE iterator insert(iterator pos, Item&& item) noexcept {
    assert(pos >= data());
    assert(pos <= data() + size());

    const auto idx = pos - data();
    reserve(size() + 1);
    const auto memory = data();

    PRAGMA_DIAGNOSTIC(push)
    PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
    std::memmove(memory + idx + 1, memory + idx, (size() - idx) * sizeof(T));
    PRAGMA_DIAGNOSTIC(pop)

    derived()->set_size(size() + 1);
    return std::construct_at(memory + idx, std::forward<Item>(item));
  }

  template <class... Args>
  PROMPP_ALWAYS_INLINE T& emplace_back(Args&&... args) noexcept {
    auto pos = size();
    reserve(pos + 1);
    derived()->set_size(pos + 1);
    return *std::construct_at(data() + pos, std::forward<Args>(args)...);
  }

  template <std::random_access_iterator IteratorType, class IteratorSentinelType>
    requires std::is_same_v<typename std::iterator_traits<IteratorType>::value_type, T> && std::sentinel_for<IteratorSentinelType, IteratorType>
  PROMPP_ALWAYS_INLINE void push_back(IteratorType begin, IteratorSentinelType end) noexcept {
    auto pos = size();
    resize(pos + std::distance(begin, end));
    std::ranges::copy(begin, end, data() + pos);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE SizeType size() const noexcept { return derived()->get_size(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const T& back() const noexcept { return data()[size() - 1]; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE T& back() noexcept { return data()[size() - 1]; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE iterator begin() noexcept { return data(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const_iterator begin() const noexcept { return data(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE iterator end() noexcept { return begin() + size(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const_iterator end() const noexcept { return begin() + size(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const T& operator[](SizeType i) const {
    assert(i < size());
    return data()[i];
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE T& operator[](SizeType i) {
    assert(i < size());
    return data()[i];
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool operator==(const GenericVector& vec) const { return size() == vec.size() && std::ranges::equal(*this, vec); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t save_size() const noexcept {
    // version is written and read by methods put() and get() and they write and read 1 byte
    return 1 + sizeof(SizeType) + sizeof(T) * size();
  }

  template <OutputStream S>
  friend S& operator<<(S& out, const GenericVector& vec) {
    auto original_exceptions = out.exceptions();
    auto sg1 = std::experimental::scope_exit([&]() { out.exceptions(original_exceptions); });
    out.exceptions(std::ifstream::failbit | std::ifstream::badbit);

    // write version
    out.put(1);

    // write size
    const auto size = vec.size();
    out.write(reinterpret_cast<const char*>(&size), sizeof(size));

    // if there are no items to write, we finish here
    if (size == 0) {
      return out;
    }

    // write data
    out.write(reinterpret_cast<const char*>(static_cast<const T*>(vec.data())), sizeof(T) * size);

    return out;
  }

  template <InputStream S>
  friend S& operator>>(S& in, GenericVector& vec) {
    assert(vec.empty());
    auto sg1 = std::experimental::scope_fail([&]() { vec.clear(); });

    // read version
    const uint8_t version = in.get();

    // return successfully, if stream is empty
    if (in.eof()) {
      return in;
    }

    // check version
    if (version != 1) [[unlikely]] {
      throw Exception(0xe637da228c04829d, "Invalid vector format version %d while reading from stream, only version 1 is supported", version);
    }

    auto original_exceptions = in.exceptions();
    auto sg2 = std::experimental::scope_exit([&]() { in.exceptions(original_exceptions); });
    in.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit);

    // read size
    SizeType size_to_read;
    in.read(reinterpret_cast<char*>(&size_to_read), sizeof(size_to_read));

    // read is completed, if there are no items
    if (!size_to_read) {
      return in;
    }

    // read data
    vec.resize(size_to_read);
    in.read(reinterpret_cast<char*>(static_cast<T*>(vec.data())), sizeof(T) * size_to_read);

    return in;
  }

 protected:
  void initialize(std::initializer_list<T> values) {
    reserve(values.size());

    auto item = data();
    for (auto it = values.begin(); it != values.end(); ++it, ++item) {
      std::construct_at(item, std::move(*it));
    }

    derived()->set_size(values.size());
  }

 private:
  PROMPP_ALWAYS_INLINE static void zero_memory(void* memory, SizeType size) {
    PRAGMA_DIAGNOSTIC(push)
    PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
    std::memset(memory, 0, size * sizeof(T));
    PRAGMA_DIAGNOSTIC(pop)
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Derived* derived() noexcept { return static_cast<Derived*>(this); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const Derived* derived() const noexcept { return static_cast<const Derived*>(this); }
};

template <template <class> class MemoryControlBlock, class T>
class MemoryBasedVector : public GenericVector<MemoryBasedVector<MemoryControlBlock, T>, typename MemoryControlBlock<T>::SizeType, T> {
 public:
  using MemoryType = Memory<MemoryControlBlock, T>;
  using SizeType = typename MemoryType::SizeType;
  using Base = GenericVector<MemoryBasedVector, SizeType, T>;

  MemoryBasedVector() noexcept = default;
  MemoryBasedVector(MemoryBasedVector&& other) noexcept = default;
  MemoryBasedVector(const MemoryBasedVector& other) noexcept { copy(other); }
  MemoryBasedVector(std::initializer_list<T> values) { Base::initialize(values); }
  explicit MemoryBasedVector(SizeType size) { Base::resize(size); }

  MemoryBasedVector& operator=(MemoryBasedVector&& o) noexcept = default;
  MemoryBasedVector& operator=(const MemoryBasedVector& other) noexcept {
    if (this != &other) [[likely]] {
      copy(other);
    }

    return *this;
  }

  ~MemoryBasedVector() noexcept {
    auto memory = Base::data();
    for (SizeType i = 0; i != get_size(); ++i) {
      std::destroy_at(memory++);
    }
  }

 protected:
  friend class GenericVector<MemoryBasedVector, SizeType, T>;

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto& memory() noexcept { return memory_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const auto& memory() const noexcept { return memory_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE SizeType get_size() const noexcept { return memory_.control_block().items_count; }
  PROMPP_ALWAYS_INLINE void set_size(SizeType size) noexcept { memory_.control_block().items_count = size; }

 private:
  MemoryType memory_;

  PROMPP_ALWAYS_INLINE void copy(const MemoryBasedVector& other) {
    set_size(other.get_size());

    if constexpr (IsTriviallyCopyable<T>::value) {
      memory_ = other.memory_;
    } else {
      memory_.grow_to_fit_at_least(get_size());
      for (SizeType i = 0; i != get_size(); ++i) {
        std::construct_at(memory_ + i, other[i]);
      }
    }
  }
};

template <class T>
using Vector = MemoryBasedVector<MemoryControlBlockWithItemCount, T>;

template <class T>
class SharedVector : public GenericVector<SharedVector<T>, typename SharedMemory<T>::SizeType, T> {
 public:
  using SizeType = typename SharedMemory<T>::SizeType;
  using Base = GenericVector<SharedVector, SizeType, T>;

  SharedVector() = default;
  SharedVector(const SharedVector&) = default;
  SharedVector(SharedVector&&) = default;
  SharedVector(std::initializer_list<T> values) { Base::initialize(values); }
  explicit SharedVector(SizeType size) { Base::resize(size); }

  SharedVector& operator=(const SharedVector&) = default;
  SharedVector& operator=(SharedVector&&) noexcept = default;

  [[nodiscard]] PROMPP_ALWAYS_INLINE const SharedPtr<T>& shared_ptr() const noexcept { return memory_.ptr(); }

 protected:
  friend class GenericVector<SharedVector, SizeType, T>;

  [[nodiscard]] PROMPP_ALWAYS_INLINE SharedMemory<T>& memory() noexcept { return memory_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const SharedMemory<T>& memory() const noexcept { return memory_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE SizeType get_size() const noexcept { return memory_.constructed_item_count(); }
  PROMPP_ALWAYS_INLINE void set_size(SizeType size) noexcept { memory_.set_constructed_item_count(size); }

 private:
  SharedMemory<T> memory_;
};

template <class T>
struct IsTriviallyReallocatable<Vector<T>> : std::true_type {};

template <class T>
struct IsTriviallyReallocatable<SharedVector<T>> : std::true_type {};

template <class T>
struct IsZeroInitializable<Vector<T>> : std::true_type {};

template <class T>
struct IsZeroInitializable<SharedVector<T>> : std::true_type {};

template <class T>
class SharedSpan {
 public:
  using iterator_category = std::contiguous_iterator_tag;
  using value_type = T;
  using iterator = T*;
  using const_iterator = const T*;
  using SizeType = typename SharedVector<T>::SizeType;

  SharedSpan() noexcept = default;

  template <class Item>
    requires std::is_trivially_destructible_v<Item>
  explicit SharedSpan(const SharedVector<Item>& vector) : data_(reinterpret_cast<const SharedPtr<T>&>(vector.shared_ptr())), size_(vector.size()) {}

  SharedSpan(const SharedSpan&) = default;
  SharedSpan(SharedSpan&& other) noexcept : data_(std::move(other.data_)), size_(std::exchange(other.size_, 0)) {}
  SharedSpan& operator=(const SharedSpan&) = default;
  SharedSpan& operator=(SharedSpan&& other) noexcept {
    if (this != other) [[likely]] {
      data_ = std::move(other.data_);
      size_ = std::exchange(other.size_, 0);
    }

    return *this;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const T& operator[](SizeType i) const {
    assert(i < size_);
    return data_.get()[i];
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE T& operator[](SizeType i) {
    assert(i < size_);
    return data_.get()[i];
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE SizeType size() const noexcept { return size_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const T* data() const noexcept { return begin(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const T* begin() const noexcept { return data_.get(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const T* end() const noexcept { return begin() + size_; }

 private:
  SharedPtr<T> data_;
  SizeType size_{};
};

}  // namespace BareBones
