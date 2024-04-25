#pragma once

#include "prometheus/selector.h"

namespace series_index::querier {

inline std::string_view MatcherTypeToString(PromPP::Prometheus::LabelMatcher::Type type) {
  using LabelMatcher = PromPP::Prometheus::LabelMatcher;

  switch (type) {
    case LabelMatcher::Type::kExactMatch:
      return "kExactMatch";
    case LabelMatcher::Type::kExactNotMatch:
      return "kExactNotMatch";
    case LabelMatcher::Type::kRegexpMatch:
      return "kRegexpMatch";
    case LabelMatcher::Type::kRegexpNotMatch:
      return "kRegexpNotMatch";
    case LabelMatcher::Type::kUnknown:
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

inline std::ostream& operator<<(std::ostream& stream, const PromPP::Prometheus::Selector::MatchResult& match_result) {
  stream << "result: { matches: { ";
  for (auto it = match_result.matches.begin(); it != match_result.matches.end(); ++it) {
    if (it != match_result.matches.begin()) {
      stream << ", ";
    }
    stream << *it;
  }
  stream << " }, status: " << MatchStatusToString(match_result.status) << "}, label_name_id: " << match_result.label_name_id
         << ", cardinality: " << match_result.cardinality;
  return stream;
}

inline std::ostream& operator<<(std::ostream& stream, const PromPP::Prometheus::Selector& selector) {
  stream << "matchers: {";
  for (auto it = selector.matchers.begin(); it != selector.matchers.end(); ++it) {
    if (it != selector.matchers.begin()) {
      stream << ", ";
    }
    stream << "{\n"
           << "matcher: {" << it->matcher << "}\nresult: {" << it->result << "}\n}";
  }
  stream << "}";
  return stream;
}

}  // namespace series_index::querier