#pragma once

#include <algorithm>
#include <limits>
#include <string>
#include <vector>

#include "bare_bones/preprocess.h"

namespace PromPP::Prometheus {

enum class MatcherType : uint8_t {
  kExactMatch = 0,
  kExactNotMatch,
  kRegexpMatch,
  kRegexpNotMatch,
  kUnknown,
};

[[nodiscard]] PROMPP_ALWAYS_INLINE MatcherType MatcherTypeFromInt(int32_t type) noexcept {
  if (type < static_cast<int32_t>(MatcherType::kExactMatch) || type >= static_cast<int32_t>(MatcherType::kUnknown)) {
    return MatcherType::kUnknown;
  }

  return static_cast<MatcherType>(type);
}

template <class StringType>
struct LabelMatcherTrait {
  StringType name{};
  StringType value{};
  MatcherType type{MatcherType::kUnknown};

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_valid() const noexcept { return !name.empty() && !value.empty() && type != MatcherType::kUnknown; }

  PROMPP_ALWAYS_INLINE void invalidate() noexcept { type = MatcherType::kUnknown; }

  PROMPP_ALWAYS_INLINE void set_default_protobuf_values() noexcept { type = MatcherType::kExactMatch; }

  auto operator<=>(const LabelMatcherTrait&) const noexcept = default;
};

using LabelMatcher = LabelMatcherTrait<std::string>;
using LabelMatchers = std::vector<LabelMatcher>;

enum class MatchStatus : uint8_t { kUnknown = 0, kEmptyMatch, kAllMatch, kPartialMatch, kError };

struct Selector {
  struct Matcher {
    std::vector<uint32_t> matches{};
    uint32_t label_name_id{std::numeric_limits<uint32_t>::max()};
    uint32_t cardinality{};
    MatchStatus status{MatchStatus::kUnknown};
    MatcherType type{MatcherType::kUnknown};

    auto operator<=>(const Matcher&) const noexcept = default;

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const noexcept { return matches.empty(); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_positive() const noexcept { return type == MatcherType::kExactMatch || type == MatcherType::kRegexpMatch; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_negative() const noexcept { return type == MatcherType::kExactNotMatch || type == MatcherType::kRegexpNotMatch; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_unknown() const noexcept { return type == MatcherType::kUnknown; }

    PROMPP_ALWAYS_INLINE void convert_to_negative() noexcept {
      if (type == MatcherType::kExactMatch) {
        type = MatcherType::kExactNotMatch;
      } else if (type == MatcherType::kRegexpMatch) {
        type = MatcherType::kRegexpNotMatch;
      }
    }
  };

  std::vector<Matcher> matchers;

  bool operator<=>(const Selector&) const noexcept = default;

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const noexcept { return matchers.empty(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool have_positive_matchers() const noexcept {
    return std::ranges::any_of(matchers, [](const Matcher& matcher) PROMPP_LAMBDA_INLINE { return matcher.is_positive(); });
  }
};

}  // namespace PromPP::Prometheus