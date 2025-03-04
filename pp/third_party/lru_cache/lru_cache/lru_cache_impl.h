// Internal implementation of an LRU cache.
//
// This base class can be re-used to provide the logic of an LRU cache with
// different underlying containers.
//
// The cache is based on a 2-container approach:
//  - a map of key -> IndexType.
//  - a linked list of Node, indexable by IndexType.
//
// The map allows for fast lookups, while the linked list keeps track of the
// order of access (or insertion) of the items.
//
// User-provided functions are called to fetch a new value, and (optionally)
// when a value is dropped.
//
// In addition to cached lookups, this cache provides iteration on all the
// elements in the cache in the access order.
//
// ADD AN IMPLEMENTATION
//
// In order to use this class, several constraints must be satisfied:
//  - LruCacheImpl uses the CRTP pattern, so the implementation must
//    publicly inherit from LruCacheImpl.
//  - The choice of containers is up to the implementation. It must provide
//    types (or wrappers) that conform to the limited implementation of the
//    Map and NodeContainer (see below).
//
// Required implementation interface:
//  - You probably want to declare the LruCacheImpl a friend class to make the
//    rest of the interface private.
//
//  size_t max_size() const;
//  NodeContainer &node_container();
//  Map &map();
//  const Map &map() const;
//  IndexType index_of(const Key &key) const;
//
// Required Map interface:
//  size_t size() const;
//  bool empty() const;
//  void emplace(const Key &key, IndexType index);
//  void erase(const Key &key);
//
// Required NodeContainer interface:
//  // INVALID_INDEX must be a valid value of IndexType. When the index is an
//  // integer, a common way to go is to reserve the max value to be invalid,
//  // although it reduces the max number of indexable values by 1.
//  static constexpr IndexType INVALID_INDEX = ...;
//  using node_type = internal::Node<Key, Value, IndexType>;
//
//  node_type &operator[](IndexType index);
//  const node_type &operator[](IndexType index) const;
//
//  // Add the node at the back. This is called while the container
//  // is not full yet, so no replacement occurs.
//  IndexType emplace_back(node_type node);
//
//  // Replace the entry at the given index with the given key with the new
//  // node. This implies that the container is full.
//  IndexType replace_entry(IndexType index, const Key &old_key,
//                          node_type new_node);
//
// Required CacheOptions interface:
//  using IndexType = ...;
//  using Node = internal::Node<Key, Value, IndexType>;
//  using Map = ...; // map of key -> IndexType
//  using NodeContainer = ...;
//  // Whether to sort the element by access order or insertion order.
//  static constexpr bool ByAccessOrder = ...;
#ifndef LRU_CACHE_LRU_CACHE_IMPL_H_
#define LRU_CACHE_LRU_CACHE_IMPL_H_

#include <cassert>
#include <functional>
#include <iterator>
#include <type_traits>

#include "default_providers.h"
#include "traits_util.h"

namespace lru_cache::internal {

// Tag to tell Node to use a Node* as linked list element.
struct self_ptr_link_tag {};

// Element in the linked list of last accessed.
template <typename Key, typename Value, typename index_type>
struct Node {
  // To allow pointer-style linked list, if index_type is self_ptr_link_tag we
  // use Node* as index type.
  using IndexType = std::conditional_t<std::is_same_v<index_type, self_ptr_link_tag>, Node*, index_type>;
  using key_type = Key;
  using value_type = Value;
  using pair_type = std::pair<Key, Value>;

  Node() = default;

  Node(Key key, Value value, IndexType prev, IndexType next) : prev_(prev), next_(next), value_pair_(std::move(key), std::move(value)) {}

  // Move-only node.
  Node(const Node&) = delete;
  Node& operator=(const Node&) = delete;
  Node(Node&&) = default;
  Node& operator=(Node&&) = default;

  Value& value() { return value_pair().second; }
  const Value& value() const { return value_pair().second; }

  Key& key() { return value_pair().first; }
  const Key& key() const { return value_pair().first; }

  pair_type& value_pair() { return value_pair_; }
  const pair_type& value_pair() const { return value_pair_; }

  // The hash of a node is just the hash of the key.
  template <typename H>
  friend H AbslHashValue(H h, const Node& n) {
    return H::combine(std::move(h), n.key());
  }

  // Link to the next newer element.
  IndexType prev_;
  // Link to the next older element.
  IndexType next_;

 private:
  // The content of the key and value.
  pair_type value_pair_;
};

// The linked list keeping track of the access order.
template <typename Key, typename Value, typename IndexType, typename NodeContainer>
struct LinkedList {
  using node_type = typename NodeContainer::node_type;
  static constexpr IndexType INVALID_INDEX = NodeContainer::INVALID_INDEX;

  LinkedList(NodeContainer& nodes) : list_content_(nodes) {}

  // The element that was access the longest ago.
  node_type& oldest() {
    assert(oldest_ != INVALID_INDEX);
    return list_content_[oldest_];
  }
  IndexType oldest_index() const { return oldest_; }

  // The element that was last access.
  node_type& latest() {
    assert(latest_ != INVALID_INDEX);
    return list_content_[latest_];
  }

  // Add a new element to the list.
  // This assumes that the list (and the underlying container) is not full.
  std::pair<std::reference_wrapper<Value>, IndexType> emplace_latest(Key key, Value value, size_t) {
    IndexType new_index = list_content_.emplace_back(node_type{std::move(key), std::move(value), INVALID_INDEX, latest_});
    if (oldest_ == INVALID_INDEX) {
      oldest_ = new_index;
    } else {
      latest().prev_ = new_index;
    }
    latest_ = new_index;
    return {latest().value(), new_index};
  }

  // Move the element to the front of the queue.
  // This doesn't move anything in memory, it just changes the pointers around.
  void move_to_front(IndexType index) {
    if (index == latest_)
      return;
    node_type& node = list_content_[index];
    // It's not the first, so it has a prev.
    assert(node.prev_ != INVALID_INDEX);
    node_type& prev_node = list_content_[node.prev_];
    prev_node.next_ = node.next_;
    if (node.next_ != INVALID_INDEX) {
      list_content_[node.next_].prev_ = node.prev_;
    } else {
      oldest_ = node.prev_;
    }
    node.next_ = latest_;
    list_content_[latest_].prev_ = index;
    node.prev_ = INVALID_INDEX;
    latest_ = index;
  }

  // Replace the oldest entry, with key old_key, with the new value and the new
  // key.
  // Depending on the container implementation, that can either re-use the
  // memory or delete and create a new one.
  const Value& replace_oldest_entry(const Key& old_key, const Key& new_key, Value new_value) {
    node_type& oldest_node = oldest();
    node_type& one_before_last = list_content_[oldest_node.prev_];
    oldest_ = oldest_node.prev_;
    IndexType oldest_node_index = one_before_last.next_;

    auto new_index = list_content_.replace_entry(oldest_node_index, old_key, node_type{new_key, std::move(new_value), INVALID_INDEX, latest_});

    latest().prev_ = new_index;
    one_before_last.next_ = INVALID_INDEX;
    latest_ = new_index;

    return list_content_[new_index].value();
  }

  void erase(IndexType index) {
    auto& node = operator[](index);
    if (node.prev_ != INVALID_INDEX) {
      operator[](node.prev_).next_ = node.next_;
    }
    if (node.next_ != INVALID_INDEX) {
      operator[](node.next_).prev_ = node.prev_;
    }

    if (oldest_ == index) {
      oldest_ = node.prev_;
    }
    if (latest_ == index) {
      latest_ = node.next_;
    }

    list_content_.erase_entry(node.key());
  }

  node_type& operator[](IndexType index) { return list_content_[index]; }
  const node_type& operator[](IndexType index) const { return list_content_[index]; }

  NodeContainer& list_content_;
  // Last element accessed.
  IndexType latest_ = INVALID_INDEX;
  // Oldest element accessed.
  IndexType oldest_ = INVALID_INDEX;
};

// Main cache implementation base.
template <
    // The class that inherits from this. See
    // https://en.wikipedia.org/wiki/Curiously_recurring_template_pattern.
    typename CRTPBase,
    // Type of the user key.
    typename Key,
    // Type of the user value.
    typename Value,
    // The struct defining the options for the cache.
    typename CacheOptions,
    // Type of the function that fetches a new value given the key. If
    // the function type is impossible to write (e.g. lambda) this can be
    // replaced by std::function, but at a performance cost.
    typename ValueProvider = decltype(&throwing_value_producer<Key, Value>),
    // The type of the function called when a value is dropped.
    typename DroppedEntryCallback = decltype(&no_op_dropped_entry_callback<Key, Value>)>
class LruCacheImpl {
 public:
  // For easy access in the derived classes.
  using Impl = LruCacheImpl;

  using IndexType = typename CacheOptions::IndexType;

  using Map = typename CacheOptions::Map;

  using NodeContainer = typename CacheOptions::NodeContainer;

  static constexpr bool ByAccessOrder = CacheOptions::ByAccessOrder;

  using node_type = typename NodeContainer::node_type;

  using value_type = typename node_type::pair_type;

  using linked_list = LinkedList<Key, Value, IndexType, NodeContainer>;

  static constexpr IndexType INVALID_INDEX = linked_list::INVALID_INDEX;

  LruCacheImpl(ValueProvider value_provider = throwing_value_producer<Key, Value>,
               DroppedEntryCallback dropped_entry_callback = no_op_dropped_entry_callback<Key, Value>)
      : value_list_(node_container()), value_provider_(std::move(value_provider)), dropped_entry_callback_(std::move(dropped_entry_callback)) {}

  // The number of elements in the cache.
  size_t size() const { return map().size(); }

  bool empty() const { return map().empty(); }

  // Whether the key is in the cache.
  bool contains(const Key& key) const { return index_of(key) != INVALID_INDEX; }

  // Get the value for the given key. If the key is not in the cache, the value
  // provider will be called to get the value, and it will be added to the
  // cache. That might cause the LRU entry to be dropped.
  const Value& operator[](const Key& key) { return get(key); }
  const Value& operator()(const Key& key) { return get(key); }

  const Value& get(const Key& key) {
    const Value* value_or_null = get_or_null(key);
    if (value_or_null != nullptr)
      return *value_or_null;
    // Fetch the value.
    Value new_value = value_provider_(key);
    return insert(key, std::move(new_value));
  }

  const Value* get_or_null(const Key& key) {
    IndexType index = index_of(key);
    if (index == INVALID_INDEX) {
      return nullptr;
    }
    node_type& node = value_list_[index];
    if constexpr (ByAccessOrder) {
      // Update the last access order.
      value_list_.move_to_front(index);
    }
    return &node.value();
  }

  const Value& insert(const Key& key, Value new_value) {
    if (size() >= max_size()) {
      // Cache is full, drop the last entry and replace it.
      return replace_oldest_entry(key, std::move(new_value));
    }
    // Append at the back of the cache.
    size_t current_size = size();
    const auto& [value, new_index] = value_list_.emplace_latest(key, std::move(new_value), current_size);
    map().emplace(key, new_index);
    return value;
  }

  bool erase(const Key& key) {
    IndexType index = index_of(key);
    if (index == INVALID_INDEX) {
      return false;
    }

    node_type& node = value_list_[index];
    map().erase(node.key());
    dropped_entry_callback_(node.key(), std::move(node.value()));
    value_list_.erase(index);
    return true;
  }

  std::pair<const Key&, const Value&> oldest() {
    auto& oldest = value_list_.oldest();
    return {oldest.key(), oldest.value()};
  }

 private:
  // Replace the oldest entry with the new entry key/new_value.
  const Value& replace_oldest_entry(const Key& key, Value new_value) {
    node_type& oldest_node = value_list_.oldest();
    Key old_key = oldest_node.key();
    map().erase(oldest_node.key());
    dropped_entry_callback_(oldest_node.key(), std::move(oldest_node.value()));
    map().emplace(key, value_list_.oldest_index());
    return value_list_.replace_oldest_entry(old_key, key, std::move(new_value));
  }

  // The specific implementation subclass.
  CRTPBase& base() { return *static_cast<CRTPBase*>(this); }
  const CRTPBase& base() const { return *static_cast<const CRTPBase*>(this); }

  size_t max_size() const { return base().max_size(); }

  NodeContainer& node_container() { return base().node_container(); }

  Map& map() { return base().map(); }
  const Map& map() const { return base().map(); }

  IndexType index_of(const Key& key) const { return base().index_of(key); }

 public:
  // Iterator, to go through the entries in access/insertion order (potentially
  // reversed).
  template <typename IteratorValueType, bool Reversed>
  class Iterator {
   public:
    using difference_type = std::ptrdiff_t;
    using value_type = IteratorValueType;
    using pointer = IteratorValueType*;
    using reference = IteratorValueType&;
    using iterator_category = std::bidirectional_iterator_tag;

    Iterator() = default;

    Iterator(const linked_list& list, IndexType index) : linked_list_(&list), current_index_(index) {}

    const IteratorValueType& operator*() const { return node().value_pair(); }
    const IteratorValueType* operator->() const { return &node().value_pair(); }

    Iterator& operator++() {
      to_next();
      return *this;
    }

    Iterator operator++(int) {
      Iterator tmp = *this;
      ++*this;
      return tmp;
    }

    Iterator& operator--() {
      to_prev();
      return *this;
    }

    Iterator operator--(int) {
      Iterator tmp = *this;
      --*this;
      return tmp;
    }

    bool operator==(const Iterator& other) const {
      if (other.current_index_ == INVALID_INDEX && current_index_ == INVALID_INDEX) {
        return true;
      }
      return other.linked_list_ == linked_list_ && other.current_index_ == current_index_;
    }

    bool operator!=(const Iterator& other) const { return !(*this == other); }

   private:
    void to_next() {
      if constexpr (Reversed) {
        current_index_ = node().prev_;
      } else {
        current_index_ = node().next_;
      }
    }

    void to_prev() {
      if constexpr (Reversed) {
        current_index_ = node().next_;
      } else {
        current_index_ = node().prev_;
      }
    }

    const node_type& node() const {
      assert(current_index_ != INVALID_INDEX);
      return (*linked_list_)[current_index_];
    }
    const linked_list* linked_list_;
    IndexType current_index_;
  };

  using iterator = Iterator<value_type, false>;
  using const_iterator = Iterator<const value_type, false>;
  using reversed_iterator = Iterator<value_type, true>;
  using const_reversed_iterator = Iterator<const value_type, true>;

  iterator begin() {
    if (size() == 0)
      return {value_list_, INVALID_INDEX};
    return {value_list_, value_list_.latest_};
  }

  iterator end() { return {value_list_, INVALID_INDEX}; }

  const_iterator begin() const {
    if (size() == 0)
      return {value_list_, INVALID_INDEX};
    return {value_list_, value_list_.latest_};
  }

  const_iterator end() const { return {value_list_, INVALID_INDEX}; }

  reversed_iterator rbegin() {
    if (size() == 0)
      return {value_list_, INVALID_INDEX};
    return {value_list_, value_list_.oldest_};
  }

  reversed_iterator rend() { return {value_list_, INVALID_INDEX}; }

  const_reversed_iterator rbegin() const {
    if (size() == 0)
      return {value_list_, INVALID_INDEX};
    return {value_list_, value_list_.oldest_};
  }

  const_reversed_iterator rend() const { return {value_list_, INVALID_INDEX}; }

  // Find an element in the cache.
  template <typename K>
  iterator find(const K& key) {
    IndexType index = index_of(key);
    return {value_list_, index};
  }

  template <typename K>
  const_iterator find(const K& key) const {
    IndexType index = index_of(key);
    return {value_list_, index};
  }

 private:
  linked_list value_list_;
  ValueProvider value_provider_;
  DroppedEntryCallback dropped_entry_callback_;
};

}  // namespace lru_cache::internal

namespace std {
// The hash of a node is just the hash of the key.
template <typename Key, typename Value, typename IndexType>
struct hash<lru_cache::internal::Node<Key, Value, IndexType>> {
  std::size_t operator()(const lru_cache::internal::Node<Key, Value, IndexType>& node) const { return hash<Key>()(node.key()); }
};
}  // namespace std

#endif  // LRU_CACHE_LRU_CACHE_IMPL_H_
