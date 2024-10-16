#pragma once

#include "bare_bones/xxhash.h"

namespace PromPP::Primitives::hash {

template <class LabelSet>
size_t hash_of_label_set(const LabelSet& label_set) noexcept {
  BareBones::XXHash hash;
  for (const auto& [label_name, label_value] : label_set) {
    hash.extend(static_cast<std::string_view>(label_name), static_cast<std::string_view>(label_value));
  }
  return static_cast<size_t>(hash);
}

template <class StringList>
size_t hash_of_string_list(const StringList& strings) noexcept {
  BareBones::XXHash hash;
  for (const auto& string : strings) {
    hash.extend(static_cast<std::string_view>(string));
  }
  return static_cast<size_t>(hash);
}

}  // namespace PromPP::Primitives::hash