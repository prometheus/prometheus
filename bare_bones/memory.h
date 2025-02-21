#pragma once

#include <cstring>

#include "preprocess.h"
#include "type_traits.h"

namespace BareBones {

template <template <class> class DerivedMemory, class T>
class GenericMemory {
 public:
  static_assert(IsTriviallyReallocatable<T>::value, "type parameter of this class should be trivially reallocatable");

  using value_type = T;
  using iterator = T*;
  using const_iterator = const T*;

  using Derived = DerivedMemory<T>;

  PROMPP_ALWAYS_INLINE void resize_to_fit_at_least(size_t needed_size) noexcept { derived()->resize(get_allocation_size(needed_size)); }

  PROMPP_ALWAYS_INLINE void grow_to_fit_at_least(size_t needed_size) noexcept {
    if (needed_size > size()) {
      resize_to_fit_at_least(needed_size);
    }
  }

  PROMPP_ALWAYS_INLINE void grow_to_fit_at_least_and_fill_with_zeros(size_t needed_size) noexcept {
    if (needed_size > size()) {
      size_t old_size = size();
      resize_to_fit_at_least(needed_size);
      std::memset(begin() + old_size, 0, (size() - old_size) * sizeof(T));
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool empty() const noexcept { return size() == 0; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept { return derived()->get_size(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const T* begin() const noexcept { return derived()->data(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE T* begin() noexcept { return derived()->data(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const T* end() const noexcept { return derived()->data() + size(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE T* end() noexcept { return derived()->data() + size(); }

  // NOLINTNEXTLINE(google-explicit-constructor)
  [[nodiscard]] PROMPP_ALWAYS_INLINE operator const T*() const noexcept { return begin(); }

  // NOLINTNEXTLINE(google-explicit-constructor)
  [[nodiscard]] PROMPP_ALWAYS_INLINE operator T*() noexcept { return begin(); }

 private:
  [[nodiscard]] PROMPP_ALWAYS_INLINE static size_t get_allocation_size(size_t needed_size) noexcept {
    static constexpr size_t kMinAllocationSize = 32;

    if (needed_size > std::numeric_limits<uint32_t>::max()) [[unlikely]] {
      std::abort();
    }

    if (sizeof(T) < 8 && static_cast<double>(needed_size) * 1.5 * sizeof(T) < 256) {
      // grow 50%, round up to 32b
      return ((static_cast<size_t>(needed_size * sizeof(T) * 1.5) & 0xFFFFFFFFFFFFFFE0) + kMinAllocationSize) / sizeof(T);
    }

    if (static_cast<double>(needed_size) * 1.5 * sizeof(T) < 4096) {
      // grow 50%, round up to 256b
      return ((static_cast<size_t>(needed_size * sizeof(T) * 1.5) & 0xFFFFFFFFFFFFFF00) + 256) / sizeof(T);
    }

    // grow 10%, round up to 4096b
    const auto new_size = ((static_cast<size_t>(needed_size * sizeof(T) * 1.1) & 0xFFFFFFFFFFFFF000) + 4096) / sizeof(T);
    return std::min(new_size, static_cast<size_t>(std::numeric_limits<uint32_t>::max()));
  }

  PROMPP_ALWAYS_INLINE Derived* derived() noexcept { return static_cast<Derived*>(this); }
  PROMPP_ALWAYS_INLINE const Derived* derived() const noexcept { return static_cast<const Derived*>(this); }
};

template <class T>
class Memory : public GenericMemory<Memory, T> {
 public:
  PROMPP_ALWAYS_INLINE Memory() noexcept = default;
  PROMPP_ALWAYS_INLINE Memory(const Memory& o) noexcept { copy(o); }
  PROMPP_ALWAYS_INLINE Memory(Memory&& o) noexcept : data_(std::exchange(o.data_, nullptr)), size_(std::exchange(o.size_, 0)) {}
  PROMPP_ALWAYS_INLINE ~Memory() noexcept { std::free(data_); }

  PROMPP_ALWAYS_INLINE Memory& operator=(const Memory& o) noexcept {
    if (this != &o) [[likely]] {
      copy(o);
    }

    return *this;
  }

  PROMPP_ALWAYS_INLINE Memory& operator=(Memory&& o) noexcept {
    if (this != &o) [[likely]] {
      std::free(data_);

      size_ = std::exchange(o.size_, 0);
      data_ = std::exchange(o.data_, nullptr);
    }

    return *this;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return size_ * sizeof(T); }

 protected:
  friend class GenericMemory<Memory, T>;

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t get_size() const noexcept { return size_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE T* data() noexcept { return data_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const T* data() const noexcept { return data_; }

  PROMPP_ALWAYS_INLINE void resize(size_t new_size) noexcept {
    PRAGMA_DIAGNOSTIC(push)
    PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
    data_ = static_cast<T*>(std::realloc(data_, new_size * sizeof(T)));
    PRAGMA_DIAGNOSTIC(pop)

    if (data_ == nullptr) [[unlikely]] {
      std::abort();
    }

    size_ = new_size;
  }

 private:
  T* data_{};
  uint32_t size_{};

  PROMPP_ALWAYS_INLINE void copy(const Memory& o) noexcept {
    static_assert(IsTriviallyCopyable<T>::value, "it's not allowed to copy memory for non trivially copyable types");

    resize(o.size_);

    PRAGMA_DIAGNOSTIC(push)
    PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
    std::memcpy(data_, o.data_, size_ * sizeof(T));
    PRAGMA_DIAGNOSTIC(pop)
  }
};

template <class T>
class SharedPtr {
 public:
  using RefCounter = uint32_t;
  using ItemCounter = uint32_t;
  using AtomicRefCounter = std::atomic<RefCounter>;

  struct ControlBlock {
    RefCounter ref_count{1};
    ItemCounter constructed_item_count{};

    [[nodiscard]] PROMPP_ALWAYS_INLINE AtomicRefCounter& atomic_ref_count() noexcept { return reinterpret_cast<AtomicRefCounter&>(ref_count); }
  };

  static constexpr uint32_t kControlBlockSize = sizeof(ControlBlock);

  SharedPtr() = default;
  explicit SharedPtr(uint32_t size) : data_(nullptr) { non_atomic_reallocate(size); }
  SharedPtr(const SharedPtr& other) noexcept : data_(other.data_) { inc_atomic_ref_counter(); }
  SharedPtr(SharedPtr&& other) noexcept : data_(std::exchange(other.data_, nullptr)) {}

  PROMPP_ALWAYS_INLINE ~SharedPtr() { dec_ref_counter(); }

  PROMPP_ALWAYS_INLINE SharedPtr& operator=(const SharedPtr& other) noexcept {
    if (this != &other) [[likely]] {
      dec_ref_counter();
      data_ = other.data_;
      inc_atomic_ref_counter();
    }

    return *this;
  }

  PROMPP_ALWAYS_INLINE SharedPtr& operator=(SharedPtr&& other) noexcept {
    if (this != &other) [[likely]] {
      dec_ref_counter();
      data_ = std::exchange(other.data_, nullptr);
    }

    return *this;
  }

  PROMPP_ALWAYS_INLINE void non_atomic_reallocate(uint32_t size) noexcept {
    if (size <= constructed_item_count()) [[unlikely]] {
      return;
    }

    PRAGMA_DIAGNOSTIC(push)
    PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
    auto control_block = static_cast<ControlBlock*>(std::realloc(raw_memory(), kControlBlockSize + size * sizeof(T)));
    PRAGMA_DIAGNOSTIC(pop)

    if (data_ == nullptr) [[likely]] {
      std::construct_at(control_block);
    } else {
      control_block->ref_count = 1;
    }

    data_ = reinterpret_cast<T*>(control_block + 1);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE RefCounter non_atomic_ref_count() const noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      return block->ref_count;
    }

    return 0;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool non_atomic_is_unique() const noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      return block->ref_count == 1;
    }

    return true;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE ItemCounter constructed_item_count() const noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      return block->constructed_item_count;
    }

    return 0;
  }

  PROMPP_ALWAYS_INLINE void set_constructed_item_count(ItemCounter count) noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      block->constructed_item_count = count;
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE T* get() const noexcept { return data_; }

  PROMPP_ALWAYS_INLINE void swap(SharedPtr& other) noexcept { std::swap(data_, other.data_); }

 private:
  T* data_{nullptr};

  PROMPP_ALWAYS_INLINE void inc_atomic_ref_counter() noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      ++control_block()->atomic_ref_count();
    }
  }

  PROMPP_ALWAYS_INLINE void dec_ref_counter() noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      if (block->ref_count == 1) [[likely]] {
        destroy();
      } else {
        --block->atomic_ref_count();
      }
    }
  }

  PROMPP_ALWAYS_INLINE void destroy() noexcept {
    destroy_constructed_items();
    std::free(raw_memory());
    data_ = nullptr;
  }

  PROMPP_ALWAYS_INLINE void destroy_constructed_items() noexcept {
    for (T *it = reinterpret_cast<T*>(data_), *end = it + control_block()->constructed_item_count; it != end; ++it) {
      std::destroy_at(it);
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE ControlBlock* control_block() noexcept { return static_cast<ControlBlock*>(raw_memory()); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const ControlBlock* control_block() const noexcept { return static_cast<ControlBlock*>(raw_memory()); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE void* raw_memory() const noexcept { return data_ == nullptr ? nullptr : reinterpret_cast<ControlBlock*>(data_) - 1; }
};

template <class T>
class SharedMemory : public GenericMemory<SharedMemory, T> {
 public:
  SharedMemory() = default;
  SharedMemory(const SharedMemory&) = default;
  SharedMemory(SharedMemory&& other) noexcept : data_(std::move(other.data_)), size_(std::exchange(other.size_, 0)) {}

  SharedMemory& operator=(const SharedMemory&) = default;
  PROMPP_ALWAYS_INLINE SharedMemory& operator=(SharedMemory&& other) noexcept {
    if (this != &other) [[likely]] {
      data_ = std::move(other.data_);
      size_ = std::exchange(other.size_, 0);
    }

    return *this;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE typename SharedPtr<T>::ItemCounter constructed_item_count() const noexcept { return data_.constructed_item_count(); }
  PROMPP_ALWAYS_INLINE void set_constructed_item_count(typename SharedPtr<T>::ItemCounter count) noexcept { data_.set_constructed_item_count(count); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return size_ * sizeof(T) + (data_.get() != nullptr ? sizeof(SharedPtr<T>::kControlBlockSize) : 0);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const SharedPtr<T>& ptr() const noexcept { return data_; }

 protected:
  friend class GenericMemory<SharedMemory, T>;

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t get_size() const noexcept { return size_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE T* data() noexcept { return data_.get(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const T* data() const noexcept { return data_.get(); }

  PROMPP_ALWAYS_INLINE void resize(size_t new_size) noexcept {
    if (data_.non_atomic_is_unique()) [[likely]] {
      data_.non_atomic_reallocate(new_size);
    } else {
      SharedPtr<T> new_data(new_size);
      PRAGMA_DIAGNOSTIC(push)
      PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
      std::memcpy(new_data.get(), data_.get(), size_ * sizeof(T));
      PRAGMA_DIAGNOSTIC(pop)
      data_.swap(new_data);
    }

    size_ = new_size;
  }

 private:
  SharedPtr<T> data_{};
  uint32_t size_{};
};

template <class T>
struct IsTriviallyReallocatable<Memory<T>> : std::true_type {};

template <class T>
struct IsTriviallyReallocatable<SharedMemory<T>> : std::true_type {};

template <class T>
struct IsZeroInitializable<Memory<T>> : std::true_type {};

template <class T>
struct IsZeroInitializable<SharedMemory<T>> : std::true_type {};

}  // namespace BareBones
