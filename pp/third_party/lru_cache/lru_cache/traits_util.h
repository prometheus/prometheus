#ifndef LRU_CACHE_TRAITS_UTIL_H_
#define LRU_CACHE_TRAITS_UTIL_H_

#include <limits>
#include <tuple>
#include <type_traits>

namespace lru_cache::internal {

template <typename F>
struct function_info : public function_info<decltype(&F::operator())> {};

template <typename ReturnType, typename... Args>
struct function_info<ReturnType (*)(Args...)> {
  using return_type = ReturnType;
  using args_type = std::tuple<Args...>;
};

template <typename ClassType, typename ReturnType, typename... Args>
struct function_info<ReturnType (ClassType::*)(Args...) const> {
  using return_type = ReturnType;
  using args_type = std::tuple<Args...>;
};

template <typename ClassType, typename ReturnType, typename... Args>
struct function_info<ReturnType (ClassType::*)(Args...)> {
  using return_type = ReturnType;
  using args_type = std::tuple<Args...>;
};

template <typename F>
using return_t = typename function_info<F>::return_type;

template <typename F>
using args_t = typename function_info<F>::args_type;

// Resolves to the unqualified type of the only argument of F, or void
// otherwise.
template <typename F>
using single_arg_t =
    std::conditional_t<std::tuple_size_v<args_t<F>> == 1,
                       std::remove_cv_t<std::remove_reference_t<
                           std::tuple_element_t<0, args_t<F>>>>,
                       void>;

template <size_t N, typename T>
static constexpr size_t is_representable =
    N <= static_cast<size_t>(std::numeric_limits<T>::max());

template <size_t N>
using index_type_for = std::conditional_t<
    is_representable<N, uint8_t>, uint8_t,
    std::conditional_t<
        is_representable<N, uint16_t>, uint16_t,
        std::conditional_t<is_representable<N, uint32_t>, uint32_t, uint64_t>>>;

}  // namespace lru_cache::internal
#endif  // LRU_CACHE_TRAITS_UTIL_H_
