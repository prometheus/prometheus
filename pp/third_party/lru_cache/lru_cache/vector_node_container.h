#ifndef LRU_CACHE_VECTOR_NODE_CONTAINER_H_
#define LRU_CACHE_VECTOR_NODE_CONTAINER_H_

#include <vector>

namespace lru_cache {

// A node container based on a vector.
// It doesn't define a maximum size.
template <typename Node, typename index_type = typename Node::IndexType>
struct VectorNodeContainer {
  using node_type = Node;
  using key_type = typename node_type::key_type;
  using IndexType = index_type;
  static constexpr IndexType INVALID_INDEX =
      std::numeric_limits<IndexType>::max();

  IndexType emplace_back(node_type node) {
    list_content_.emplace_back(std::move(node));
    return list_content_.size() - 1;
  }

  node_type& operator[](IndexType index) { return list_content_[index]; }

  const node_type& operator[](IndexType index) const {
    return list_content_[index];
  }

  IndexType replace_entry(IndexType index, const key_type& old_key,
                          node_type new_node) {
    list_content_[index] = std::move(new_node);
    return index;
  }

  // Reserve memory for the underlying vector, to avoid reallocations.
  void reserve(IndexType size) {
    assert(size < INVALID_INDEX);
    list_content_.reserve(size);
  }

 private:
  std::vector<node_type> list_content_;
};

}  // namespace lru_cache

#endif  // LRU_CACHE_VECTOR_NODE_CONTAINER_H_
