#pragma once

#include "prometheus/selector.h"

namespace series_index::querier {

inline std::string_view MatcherTypeToString(PromPP::Prometheus::MatcherType type) {
  using enum PromPP::Prometheus::MatcherType;

  switch (type) {
    case kExactMatch:
      return "kExactMatch";
    case kExactNotMatch:
      return "kExactNotMatch";
    case kRegexpMatch:
      return "kRegexpMatch";
    case kRegexpNotMatch:
      return "kRegexpNotMatch";
    case kUnknown:
      return "kUnknown";

    default:
      assert(0);
      return "";
  }
}

inline std::string_view MatchStatusToString(PromPP::Prometheus::MatchStatus status) {
  using MatchStatus = PromPP::Prometheus::MatchStatus;

  switch (status) {
    case MatchStatus::kEmptyMatch:
      return "kEmptyMatch";
    case MatchStatus::kAllMatch:
      return "kAllMatch";
    case MatchStatus::kPartialMatch:
      return "kPartialMatch";
    case MatchStatus::kError:
      return "kError";
    case MatchStatus::kUnknown:
      return "kUnknown";

    default:
      assert(0);
      return "";
  }
}

inline std::ostream& operator<<(std::ostream& stream, const PromPP::Prometheus::LabelMatcher& matcher) {
  stream << "name: " << matcher.name << ", value: " << matcher.value << ", type: " << MatcherTypeToString(matcher.type);
  return stream;
}

inline std::ostream& operator<<(std::ostream& stream, const PromPP::Prometheus::Selector::Matcher& matcher) {
  stream << "result: { matches: { ";
  for (auto it = matcher.matches.begin(); it != matcher.matches.end(); ++it) {
    if (it != matcher.matches.begin()) {
      stream << ", ";
    }
    stream << *it;
  }
  stream << " }, status: " << MatchStatusToString(matcher.status) << "}, label_name_id: " << matcher.label_name_id << ", cardinality: " << matcher.cardinality
         << ", type: " << MatcherTypeToString(matcher.type);
  return stream;
}

inline std::ostream& operator<<(std::ostream& stream, const PromPP::Prometheus::Selector& selector) {
  stream << "matchers: {";
  for (auto it = selector.matchers.begin(); it != selector.matchers.end(); ++it) {
    if (it != selector.matchers.begin()) {
      stream << ", ";
    }
    stream << "{\n"
           << "matcher: {" << *it << "}\n}";
  }
  stream << "}";
  return stream;
}

}  // namespace series_index::querier