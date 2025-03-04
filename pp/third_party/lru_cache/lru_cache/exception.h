#ifndef LRU_CACHE_EXCEPTION_H_
#define LRU_CACHE_EXCEPTION_H_

#include <exception>
#include <iostream>
#include <sstream>
#include <type_traits>

namespace lru_cache {
namespace internal {
template <typename, typename = void>
struct is_printable : std::false_type {};

template <typename T>
struct is_printable<T, std::void_t<decltype(std::declval<std::ostream&>()
                                            << std::declval<T>())>>
    : std::true_type {};
}  // namespace internal

template <typename Key>
class KeyNotFound : public std::exception {
 public:
  KeyNotFound(const Key& key) : message_(get_message(key)) {}

  const char* what() const noexcept override { return message_.c_str(); }

 private:
  static std::string get_message(const Key& key) {
    if constexpr (internal::is_printable<Key>::value) {
      std::stringstream ss;
      ss << "LRU cache: Key not found: " << key;
      return ss.str();
    }
    return "LRU cache: Key not found";
  }
  std::string message_;
};
}  // namespace lru_cache

#endif  // LRU_CACHE_EXCEPTION_H_
