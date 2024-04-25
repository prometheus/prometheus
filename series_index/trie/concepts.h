#pragma once

#include <concepts>
#include <string_view>

namespace series_index::trie {

template <class RegexpMatchesList, class TrieTraversal>
concept RegexpMatchesListInterface = requires(RegexpMatchesList& matches, const TrieTraversal& iterator) {
  { matches.clear() };
  { matches.count() } -> std::same_as<size_t>;
  { matches.add_leaf(iterator, std::string_view()) };
  { matches.add_leaf(iterator) };
  { matches.add_node(iterator) };
  {
    matches.add_node(iterator, [](std::string_view) { return true; })
  };
  { matches.add_subnodes(iterator) };
};

template <class Trie>
concept IsInsertableTrie = requires(Trie& trie) {
  { trie.insert(std::string_view(), size_t{}) };
};

}  // namespace series_index::trie