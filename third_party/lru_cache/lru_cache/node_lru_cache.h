// LRU cache based on absl::node_hash_set.
//
// This takes advantage of the fact that the nodes are pointer-stable in a
// node_hash_set to encode the linked list directly inside the values. That way
// there is no need to keep a vector alongside the map, so there is less
// information duplication.
//
// This is supposed to mirror the way LinkedHashMap works in Java.
#ifndef LRU_CACHE_NODE_LRU_CACHE_H_
#define LRU_CACHE_NODE_LRU_CACHE_H_

#include <vector>

#include "absl/container/node_hash_set.h"
#include "lru_cache_impl.h"
#include "traits_util.h"

namespace lru_cache {

// Adaptor over the node_hash_set to provide the linked_list.
template <typename Node, typename Map>
struct MapNodeContainer {
  using node_type = Node;
  using key_type = typename node_type::key_type;
  using IndexType = typename Node::IndexType;  // Node*
  static constexpr IndexType INVALID_INDEX = nullptr;

  MapNodeContainer(Map& map) : map_(map) {}

  IndexType emplace_back(node_type node) {
    auto [it, inserted] = map_.emplace_node(std::move(node));
    assert(inserted);
    return node_to_index(*it);
  }

  IndexType replace_entry(IndexType, const key_type& old_key, node_type new_node) {
    map_.erase_node(old_key);
    return emplace_back(std::move(new_node));
  }

  void erase_entry(const key_type& key) { map_.erase_node(key); }

  node_type& operator[](IndexType index) { return index_to_node(index); }

  const node_type& operator[](IndexType index) const { return index_to_node(index); }

 private:
  IndexType node_to_index(const Node& node) const {
    // Unfortunate, but necessary.
    return const_cast<IndexType>(&node);
  }

  Node& index_to_node(IndexType ptr) const { return *ptr; }
  Map& map_;
};

// Adaptor over a node_hash_set to provide the appearance of a map of Key ->
// Node*.
// The set contains Nodes.
template <typename Key, typename Value, typename Node>
struct MapAdaptor {
  // We want to allow heterogeneous lookup to be able to lookup a key in the
  // set, as if it was a map.
  struct NodeComparator {
    // absl heterogeneous marker.
    using is_transparent = void;
    bool operator()(const Node& n1, const Node& n2) const { return n1.key() == n2.key(); }
    bool operator()(const Key& k, const Node& n) const { return k == n.key(); }
    bool operator()(const Node& n, const Key& k) const { return k == n.key(); }
  };

  // The hash of a node is just the hash of the key.
  struct TransparentHasher : public absl::container_internal::hash_default_hash<Key> {
    using is_transparent = void;
    size_t operator()(const Key& value) const { return absl::container_internal::hash_default_hash<Key>::operator()(value); }
    size_t operator()(const Node& value) const { return operator()(value.key()); }
  };

  using IndexType = typename Node::IndexType;  // Node*
  using Set = absl::node_hash_set<Node, TransparentHasher, NodeComparator>;
  static_assert(absl::container_internal::IsTransparent<typename Set::hasher>::value, "Hasher is not transparent, heterogeneous lookup won't work");
  static_assert(absl::container_internal::IsTransparent<typename Set::key_equal>::value, "Comparator is not transparent, heterogeneous lookup won't work");

  size_t size() const { return set_.size(); }
  bool empty() const { return set_.empty(); }

  using const_iterator = typename Set::const_iterator;
  const_iterator find(const Key& key) const { return set_.find(key); }
  const_iterator end() const { return set_.end(); }

  void emplace(const Key&, IndexType) {
    // Do nothing, the emplace is in the node container.
  }

  void erase(const Key&) {
    // Do nothing, the node will be erased at the right moment by the node
    // container calling erase_node.
  }

  std::pair<typename Set::iterator, bool> emplace_node(Node node) {
    // Called by the node container.
    return set_.emplace(std::move(node));
  }

  void erase_node(const Key& key) { set_.erase(key); }

 private:
  Set set_;
};

// Set of options for the set-based LRU cache.
template <typename Key, typename Value, bool by_access_order = true>
struct NodeLruCacheOptions {
  using Node = internal::Node<Key, Value, internal::self_ptr_link_tag>;
  using IndexType = Node*;
  static_assert(std::is_same_v<IndexType, typename Node::IndexType>);
  using Map = MapAdaptor<Key, Value, Node>;
  using NodeContainer = MapNodeContainer<Node, Map>;
  static constexpr bool ByAccessOrder = by_access_order;
};

// Set-based LRU cache, with the linked list baked in the values.
template <typename Key,
          typename Value,
          typename ValueProvider = decltype(&internal::throwing_value_producer<Key, Value>),
          typename DroppedEntryCallback = void (*)(Key, Value)>
class NodeLruCache : public internal::LruCacheImpl<NodeLruCache<Key, Value, ValueProvider, DroppedEntryCallback>,
                                                   Key,
                                                   Value,
                                                   NodeLruCacheOptions<Key, Value>,
                                                   ValueProvider,
                                                   DroppedEntryCallback> {
  using Base = typename NodeLruCache::Impl;
  friend Base;
  using options_type = NodeLruCacheOptions<Key, Value>;
  using IndexType = typename options_type::IndexType;
  using NodeContainer = typename options_type::NodeContainer;
  using Map = typename options_type::Map;
  using Node = typename options_type::Node;

 public:
  NodeLruCache(size_t max_size,
               ValueProvider value_provider = internal::throwing_value_producer<Key, Value>,
               DroppedEntryCallback dropped_entry_callback = internal::no_op_dropped_entry_callback<Key, Value>)
      : Base(std::move(value_provider), std::move(dropped_entry_callback)), max_size_(max_size), nodes_(map_) {}

  size_t max_size() const { return max_size_; }
  void set_max_size(size_t max_size) noexcept { max_size_ = max_size; }

 protected:
  NodeContainer& node_container() { return nodes_; }
  Map& map() { return map_; }
  const Map& map() const { return map_; }

  IndexType index_of(const Key& key) const {
    auto it = map_.find(key);
    if (it != map_.end()) {
      return const_cast<Node*>(&*it);
    }
    return NodeContainer::INVALID_INDEX;
  }

 private:
  size_t max_size_;
  Map map_;
  NodeContainer nodes_;
};

// Factory for the set-based LRU cache.
template <typename Key,
          typename Value,
          typename ValueProvider = decltype(&internal::throwing_value_producer<Key, Value>),
          typename DroppedEntryCallback = void (*)(Key, Value)>
NodeLruCache<Key, Value, ValueProvider, DroppedEntryCallback> make_node_lru_cache(size_t max_size,
                                                                                  ValueProvider v = internal::throwing_value_producer<Key, Value>,
                                                                                  DroppedEntryCallback c = internal::no_op_dropped_entry_callback<Key, Value>) {
  return {max_size, v, c};
}

// Same as above, deducing Key and Value from the single-argument function
// ValueProvider.
template <typename ValueProvider, typename DroppedEntryCallback = decltype(&internal::no_op_dropped_entry_callback_deduced<ValueProvider>)>
NodeLruCache<internal::single_arg_t<ValueProvider>, internal::return_t<ValueProvider>, ValueProvider, DroppedEntryCallback>
make_node_lru_cache_deduced(size_t max_size, ValueProvider v, DroppedEntryCallback c = internal::no_op_dropped_entry_callback_deduced<ValueProvider>) {
  return {max_size, v, c};
}

}  // namespace lru_cache

#endif  // LRU_CACHE_NODE_LRU_CACHE_H_
