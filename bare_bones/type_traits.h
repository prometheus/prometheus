#pragma once

#include <array>
#include <memory>
#include <string>
#include <tuple>
#include <type_traits>
#include <utility>
#include <vector>

namespace BareBones {

namespace Concepts {

// checks for subtract semigroup operations of one type.
template <typename T>
concept SubtractSemigroup = requires(T t1, T t2, T t3) {
  { t3 = t2 - t1 };
};

}  // namespace Concepts

// IsTriviallyReallocatable

template <class T>
struct IsTriviallyReallocatable : std::is_trivially_copyable<T> {};

template <>
struct IsTriviallyReallocatable<std::string> : std::true_type {};

template <class T1, class T2>
struct IsTriviallyReallocatable<std::pair<T1, T2>> : std::conjunction<IsTriviallyReallocatable<T1>, IsTriviallyReallocatable<T2>> {};

template <class... Ts>
struct IsTriviallyReallocatable<std::tuple<Ts...>> : std::conjunction<IsTriviallyReallocatable<Ts>...> {};

template <class T>
struct IsTriviallyReallocatable<std::vector<T>> : std::true_type {};

template <class T, size_t N>
struct IsTriviallyReallocatable<std::array<T, N>> : IsTriviallyReallocatable<T> {};

template <class T>
struct IsTriviallyReallocatable<std::unique_ptr<T>> : std::true_type {};

// IsTriviallyCopyable

template <class T>
struct IsTriviallyCopyable : std::is_trivially_copyable<T> {};

template <class T1, class T2>
struct IsTriviallyCopyable<std::pair<T1, T2>> : std::conjunction<IsTriviallyCopyable<T1>, IsTriviallyCopyable<T2>> {};

template <class... Ts>
struct IsTriviallyCopyable<std::tuple<Ts...>> : std::conjunction<IsTriviallyCopyable<Ts>...> {};

template <class T, size_t N>
struct IsTriviallyCopyable<std::array<T, N>> : IsTriviallyCopyable<T> {};

// IsZeroInitializable

template <class T>
struct IsZeroInitializable : std::is_trivial<T> {};

template <>
struct IsZeroInitializable<std::string> : std::true_type {};

template <>
struct IsZeroInitializable<std::string_view> : std::true_type {};

template <class T1, class T2>
struct IsZeroInitializable<std::pair<T1, T2>> : std::conjunction<IsZeroInitializable<T1>, IsZeroInitializable<T2>> {};

template <class... Ts>
struct IsZeroInitializable<std::tuple<Ts...>> : std::conjunction<IsZeroInitializable<Ts>...> {};

template <class T>
struct IsZeroInitializable<std::vector<T>> : std::true_type {};

template <class T, size_t N>
struct IsZeroInitializable<std::array<T, N>> : IsZeroInitializable<T> {};

template <class T>
struct IsZeroInitializable<std::unique_ptr<T>> : std::true_type {};

// IsTriviallyDestructible

template <class T>
struct IsTriviallyDestructible : std::is_trivially_destructible<T> {};

template <class T1, class T2>
struct IsTriviallyDestructible<std::pair<T1, T2>> : std::conjunction<IsTriviallyDestructible<T1>, IsTriviallyDestructible<T2>> {};

template <class... Ts>
struct IsTriviallyDestructible<std::tuple<Ts...>> : std::conjunction<IsTriviallyDestructible<Ts>...> {};

template <class T, size_t N>
struct IsTriviallyDestructible<std::array<T, N>> : IsTriviallyDestructible<T> {};

}  // namespace BareBones
