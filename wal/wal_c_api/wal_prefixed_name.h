/**
 * \file wal_prefixed_name.h
 * Contains macros for prepending specified prefix into any named entity (variable, function, etc).
 * Before #include'ing this header you must #define OKDB_WAL_API_SET_NAME with meaningful name for
 * exported API set which would be used for struct with pointers to exported functions and
 * factory function for creating proper
 * You may #define OKDB_WAL_FUNCTION_NAME_PREFIX for common prefix in your named entities (using the
 * \ref OKDB_WAL_PREFIXED_NAME() macro) or use the \ref OKDB_WAL_PREFIXED_NAME2(prefix, name)
 * for explicit prefix.
 * After that you should create an exported functions table via \ref OKDB_WAL_EXPORT_API_BEGIN,
 * \ref OKDB_WAL_EXPORT_API(funcname_without_prefixes), \ref OKDB_WAL_EXPORT_API_END.
 *
 *  Example:
 *  Consider you want to export the `int my_cool_func(int arg1, char *arg2)`. So you should make a header with contents like:
 *  ```
 *  #define OKDB_WAL_API_SET_NAME my_cool_api
 *  #include "wal_prefixed_name.h"
 *
 *  // declaration of prefixed function.
 *  int OKDB_WAL_API_PREFIXED_NAME(my_cool_func) (int arg1, char *arg2);
 *
 *  // declaration of exported API set.
 *  OKDB_WAL_EXPORT_API_BEGIN
 *  OKDB_WAL_EXPORT_API(my_cool_func)
 *  OKDB_WAL_EXPORT_API_END
 *  ```
 *
 * \note Almost all macros related to EXPORT_API codegen are available only in C++ mode
 *       (because it depends on C++ constexpr and generic features). They are not available
 *       in C mode.
 */
// #pragma once

// Prefix for all functions named via OKDB_WAL_PREFIXED_NAME(). Can be empty.
#ifndef OKDB_WAL_FUNCTION_NAME_PREFIX
#define OKDB_WAL_FUNCTION_NAME_PREFIX
#endif

// Common macros for both C and C++.
//
/// Prepends the defined function name prefix. Use it to mark the function as prefixed API.
#define OKDB_WAL_PREFIXED_NAME(name) OKDB_WAL_PREFIXED_NAME2(OKDB_WAL_FUNCTION_NAME_PREFIX, name)
/// Concatenates two tokens with macro expanding.
#define OKDB_WAL_PREFIXED_NAME2(prefix, name) OKDB_WAL_PREFIXED_NAME2_IMPL(prefix, name)
/// Concatenates two tokens _WITHOUT_ macro expanding. Use \ref OKDB_WAL_PREFIXED_NAME2 for macro
/// expansions.
#define OKDB_WAL_PREFIXED_NAME2_IMPL(prefix, name) prefix##name

#ifndef __cplusplus

// NO-OP in C mode! As they depends on C++ tuple and constexpr LiteralString.

#define OKDB_WAL_EXPORT_API_BEGIN
#define OKDB_WAL_EXPORT_API(funcname)
#define OKDB_WAL_EXPORT_API_END

#else  // __cplusplus

//
#ifndef OKDB_WAL_API_SET_NAME
#error #define the OKDB_WAL_API_SET_NAME with meaningful name for API set which would be used for struct with pointers to exported functions and vtbl factory function
#endif

#include <algorithm>
#include <tuple>

#include <stddef.h>

// guard against multiple class definition.
#ifndef OKDB_WAL_API_DETAIL_CONST_STR
#define OKDB_WAL_API_DETAIL_CONST_STR

namespace Detail {
// helper for template string literals args
template <size_t N>
struct LiteralString {
  char str_data[N];
  // the ctor must be implicit to allow the init of LiteralString from string literal.
  // NOLINTNEXTLINE(google-explicit-constructor)
  constexpr LiteralString(const char (&lit)[N]) { std::copy_n(lit, N, str_data); }
  constexpr auto begin() const noexcept { return std::ranges::begin(str_data); }
  constexpr auto end() const noexcept { return std::ranges::end(str_data); }
  constexpr size_t size() const noexcept { return N; }
  constexpr auto data() const noexcept { return &str_data[0]; }
  template <size_t N2>
  constexpr bool operator==(const LiteralString<N2>& const_str2) const noexcept {
    return N2 == N && std::equal(this->begin(), this->end(), const_str2.begin());
  }
};
// deduction guide
template <size_t N>
LiteralString(const char (&str_arr)[N]) -> LiteralString<N>;

// Generic helper for storing function pointer and its name
template <LiteralString Name_, auto FnPtr_>
struct NamedFunction {
  constexpr auto name() const noexcept { return Name_; }

  template <typename... Args>
  constexpr decltype(auto) operator()(Args&&... args) const {
    return FnPtr_(std::forward<Args>(args)...);
  }
};
}  // namespace Detail
#endif  // OKDB_WAL_API_DETAIL_CONST_STR

// helper for concatenating 3 tokens
#define OKDB_WAL_PREFIXED_NAME3(prefix1, prefix2, name) OKDB_WAL_PREFIXED_NAME3_IMPL(prefix1, prefix2, name)
#define OKDB_WAL_PREFIXED_NAME3_IMPL(prefix1, prefix2, name) prefix1##prefix2##name

// names for generated vtbl struct and factory function
#define OKDB_WAL_VTBL_STRUCT_NAME OKDB_WAL_PREFIXED_NAME3(OKDB_WAL_FUNCTION_NAME_PREFIX, OKDB_WAL_API_SET_NAME, _api_vtbl)

// for brevity...
#define SZ(tup) std::tuple_size_v<decltype(tup)>

// Helper for saving function pointers with name
#define OKDB_WAL_FN_NAME_UNPACKED(name) OKDB_WAL_FN_NAME(name)
#define OKDB_WAL_FN_NAME_WITH_PREFIX(name) \
  Detail::NamedFunction<#name, &name> {}

#define OKDB_WAL_FN_NAME(name) OKDB_WAL_FN_NAME_WITH_OPT_PREFIX(OKDB_WAL_FUNCTION_NAME_PREFIX, name)
#define OKDB_WAL_FN_NAME_WITH_OPT_PREFIX(prefix, name) OKDB_WAL_FN_NAME_WITH_PREFIX_IMPL(prefix, name)
#define OKDB_WAL_FN_NAME_WITH_PREFIX_IMPL(prefix, name) \
  Detail::NamedFunction<#name, &prefix##name> {}

// main struct for storing generic pointers to functions (definition begin)
#define OKDB_WAL_EXPORT_API_BEGIN OKDB_WAL_EXPORT_API_BEGIN_IMPL
#define OKDB_WAL_EXPORT_API_BEGIN_IMPL \
  struct OKDB_WAL_VTBL_STRUCT_NAME {   \
    static constexpr std::tuple tup {
// Export macro for prefixed functions into vtbl
#define OKDB_WAL_EXPORT_API(name) OKDB_WAL_FN_NAME_WITH_OPT_PREFIX(OKDB_WAL_FUNCTION_NAME_PREFIX, name),

#define OKDB_WAL_EXPORT_API_END                                                                                                        \
  }                                                                                                                                    \
  ; /* std::tuple with named pointers */                                                                                               \
  template <Detail::LiteralString Name, size_t Id_>                                                                                    \
  struct NameToId {                                                                                                                    \
    static constexpr size_t Id = std::get<Id_>(tup).name() == Name ? Id_ : SZ(tup);                                                    \
  };                                                                                                                                   \
                                                                                                                                       \
  template <Detail::LiteralString Name, size_t Index>                                                                                  \
  struct FindIdByName {                                                                                                                \
    static constexpr size_t Id = NameToId<Name, Index>::Id == SZ(tup) ? FindIdByName<Name, Index + 1>::Id : NameToId<Name, Index>::Id; \
  };                                                                                                                                   \
                                                                                                                                       \
  template <Detail::LiteralString Name>                                                                                                \
  struct FindIdByName<Name, SZ(tup)> {                                                                                                 \
    /* end of compile-time ID search */                                                                                                \
    static constexpr size_t Id = SZ(tup);                                                                                              \
  };                                                                                                                                   \
                                                                                                                                       \
  /* Searcher, helper class */                                                                                                         \
  template <Detail::LiteralString Name>                                                                                                \
  using IdFinder = FindIdByName<Name, 0>;                                                                                              \
                                                                                                                                       \
  template <Detail::LiteralString Name, typename... Args>                                                                              \
  constexpr decltype(auto) call(Args&&... args) const {                                                                                \
    static_assert(IdFinder<Name>::Id != SZ(tup), "Function is not found");                                                             \
    return std::get<IdFinder<Name>::Id>(tup)(std::forward<Args>(args)...);                                                             \
  }                                                                                                                                    \
  }                                                                                                                                    \
  ;

#endif  // __cplusplus
