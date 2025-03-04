#ifndef LRU_CACHE_ARRAY_NODE_CONTAINER_H_
#define LRU_CACHE_ARRAY_NODE_CONTAINER_H_

#include <limits>
#include <type_traits>

namespace lru_cache {

// A node container based on a static array of size N, similarly to std::array.
// The memory size is fixed, there is no insertion/deletion.
template <typename Node, size_t N,
          typename index_type = typename Node::IndexType>
struct ArrayNodeContainer {
  using node_type = Node;
  using key_type = typename node_type::key_type;
  using IndexType = index_type;
  static constexpr IndexType INVALID_INDEX =
      std::numeric_limits<IndexType>::max();

  static_assert(N < INVALID_INDEX);

  ArrayNodeContainer() = default;
  // ArrayNodeContainer contains the whole array inline, therefore it can
  // neither be cheaply copied nor moved. If you need a static amount of
  // memory, but on the heap, use a VectorNodeContainer and call reserve().
  ArrayNodeContainer(const ArrayNodeContainer& other) = delete;
  ArrayNodeContainer& operator=(const ArrayNodeContainer& other) = delete;

  IndexType emplace_back(node_type node) {
    IndexType index = size_;
    ++size_;
    new (&operator[](index)) node_type(std::move(node));
    return index;
  }

  // Only destroy the ones that were initialized.
  ~ArrayNodeContainer() {
    for (size_t i = 0; i < size_; ++i) {
      operator[](i).~node_type();
    }
  }

  IndexType replace_entry(IndexType index, const key_type& old_key,
                          node_type new_node) {
    operator[](index) = std::move(new_node);
    return index;
  }

  node_type& operator[](IndexType index) {
    assert(index < size_);
    return list_content()[index];
  }
  const node_type& operator[](IndexType index) const {
    assert(index < size_);
    return list_content()[index];
  }

 private:
  node_type* list_content() { return reinterpret_cast<node_type*>(&storage_); }
  const node_type* list_content() const {
    return reinterpret_cast<node_type*>(&storage_);
  }
  // We need to keep track of the size to destroy only the elements that were
  // constructed.
  size_t size_ = 0;
  // We don't use an array or std::array because they call all the destructors
  // by default.
  std::aligned_storage_t<sizeof(node_type[N]), alignof(node_type[N])> storage_;
  static_assert(sizeof(storage_) >= N * sizeof(node_type));
};

}  // namespace lru_cache

#endif  // LRU_CACHE_ARRAY_NODE_CONTAINER_H_
