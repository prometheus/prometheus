#pragma once

#include <algorithm>
#include <limits>
#include <string>
#include <vector>

#include "bare_bones/preprocess.h"

namespace PromPP::Prometheus {

struct LabelMatcher {
  enum class Type : uint8_t { kExactMatch = 0, kExactNotMatch, kRegexpMatch, kRegexpNotMatch, kUnknown };

  std::string name{};
  std::string value{};
  Type type{Type::kUnknown};

  [[nodiscard]] PROMPP_ALWAYS_INLINE static Type as_type(int32_t type) noexcept {
    if (type < static_cast<int32_t>(Type::kExactMatch) || type >= static_cast<int32_t>(Type::kUnknown)) {
      return Type::kUnknown;
    }

    return static_cast<Type>(type);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_positive() const noexcept { return type == Type::kExactMatch || type == Type::kRegexpMatch; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_negative() const noexcept { return type == Type::kExactNotMatch || type == Type::kRegexpNotMatch; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_unknown() const noexcept { return type == Type::kUnknown; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_valid() const noexcept { return !name.empty() && !value.empty() && !is_unknown(); }

  PROMPP_ALWAYS_INLINE void convert_to_negative() noexcept {
    if (type == Type::kExactMatch) {
      type = Type::kExactNotMatch;
    } else if (type == Type::kRegexpMatch) {
      type = Type::kRegexpNotMatch;
    }
  }

  PROMPP_ALWAYS_INLINE void invalidate() noexcept { type = Type::kUnknown; }

  auto operator<=>(const LabelMatcher&) const noexcept = default;
};

enum class MatchStatus : uint8_t { kUnknown = 0, kEmptyMatch, kAllMatch, kPartialMatch, kError };

struct Selector {
  struct MatchResult {
    std::vector<uint32_t> matches{};
    uint32_t label_name_id{std::numeric_limits<uint32_t>::max()};
    uint32_t cardinality{};
    MatchStatus status{MatchStatus::kUnknown};

    auto operator<=>(const MatchResult&) const noexcept = default;

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const noexcept { return matches.empty(); }
  };

  struct Matcher {
    LabelMatcher matcher;
    MatchResult result{};

    auto operator<=>(const Matcher&) const noexcept = default;
  };

  using Matchers = std::vector<Matcher>;

  Matchers matchers;

  bool operator<=>(const Selector&) const noexcept = default;

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const noexcept { return matchers.empty(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool have_positive_matchers() const noexcept {
    return std::any_of(matchers.begin(), matchers.end(), [](const Matcher& matcher) PROMPP_LAMBDA_INLINE { return matcher.matcher.is_positive(); });
  }
};

}  // namespace PromPP::Prometheus