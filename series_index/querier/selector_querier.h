#pragma once

#include <cassert>

#include "prometheus/label_matcher.h"
#include "regexp_searcher.h"
#include "series_index/trie_index.h"

namespace series_index::querier {

enum class QuerierStatus : uint32_t {
  kNoPositiveMatchers = 0,
  kRegexpError,
  kNoMatch,
  kMatch,
};

template <class TrieIndex>
class SelectorQuerier {
 public:
  using MatcherType = PromPP::Prometheus::MatcherType;
  using Selector = PromPP::Prometheus::Selector;
  using MatchStatus = PromPP::Prometheus::MatchStatus;

  explicit SelectorQuerier(const TrieIndex& index) : index_(index) {}

  template <class LabelMatchers>
  [[nodiscard]] QuerierStatus query(const LabelMatchers& label_matchers, Selector& selector) {
    selector.matchers.reserve(label_matchers.size());

    for (auto& label_matcher : label_matchers) {
      auto& matcher = selector.matchers.emplace_back();
      matcher.type = label_matcher.type;

      if (matcher.is_positive()) {
        if (auto status = query(label_matcher, matcher); status != QuerierStatus::kMatch) {
          return status;
        }
      }
    }

    if (!selector.have_positive_matchers()) {
      return QuerierStatus::kNoPositiveMatchers;
    }

    for (size_t i = 0; i < selector.matchers.size(); ++i) {
      auto& label_matcher = label_matchers[i];
      auto& matcher = selector.matchers[i];

      if (matcher.is_negative() && matcher.status == MatchStatus::kUnknown) {
        if (auto status = query(label_matcher, matcher); status != QuerierStatus::kMatch && status != QuerierStatus::kNoMatch) {
          return status;
        }

        if (matcher.is_unknown()) {
          return QuerierStatus::kNoMatch;
        }
      }
    }

    return QuerierStatus::kMatch;
  }

  template <class LabelMatcher>
  PROMPP_ALWAYS_INLINE QuerierStatus query(const LabelMatcher& label_matcher, Selector::Matcher& matcher) {
    return query_values(label_matcher, get_values_trie(label_matcher, matcher), matcher);
  }

 private:
  const TrieIndex& index_;

  template <class LabelMatcher>
  PROMPP_ALWAYS_INLINE const TrieIndex::Trie* get_values_trie(const LabelMatcher& label_matcher, Selector::Matcher& matcher) const noexcept {
    if (auto index = index_.names_trie().lookup(static_cast<std::string_view>(label_matcher.name)); index) {
      matcher.label_name_id = *index;
      return index_.values_trie(*index);
    }

    return nullptr;
  }

  template <class LabelMatcher>
  QuerierStatus query_values(const LabelMatcher& label_matcher, const TrieIndex::Trie* trie, Selector::Matcher& matcher) {
    if (label_matcher.value.empty()) {
      process_empty_matcher(matcher, trie);
      return QuerierStatus::kMatch;
    }

    switch (matcher.type) {
      case MatcherType::kExactMatch:
      case MatcherType::kExactNotMatch: {
        return query_exact_value(label_matcher, trie, matcher);
      }

      case MatcherType::kRegexpMatch:
      case MatcherType::kRegexpNotMatch: {
        return query_values_by_regexp(label_matcher, trie, matcher);
      }

      default: {
        assert(false);
        return QuerierStatus::kNoMatch;
      }
    }
  }

  template <class LabelMatcher>
  QuerierStatus query_exact_value(const LabelMatcher& label_matcher, const TrieIndex::Trie* trie, Selector::Matcher& matcher) {
    if (trie == nullptr) {
      matcher.status = MatchStatus::kEmptyMatch;
      return QuerierStatus::kNoMatch;
    }

    if (auto value = trie->lookup(static_cast<std::string_view>(label_matcher.value)); value) {
      matcher.matches.emplace_back(*value);
      matcher.status = MatchStatus::kPartialMatch;
      return QuerierStatus::kMatch;
    }

    matcher.status = MatchStatus::kEmptyMatch;
    return QuerierStatus::kNoMatch;
  }

  void process_empty_matcher(Selector::Matcher& matcher, const TrieIndex::Trie* trie) {
    if (matcher.is_positive()) {
      matcher.convert_to_negative();

      if (trie != nullptr) {
        matcher.status = MatchStatus::kAllMatch;
      } else {
        matcher.status = MatchStatus::kEmptyMatch;
      }
    } else {
      if (trie != nullptr) {
        matcher.status = MatchStatus::kAllMatch;
      } else {
        matcher.status = MatchStatus::kEmptyMatch;
        matcher.type = MatcherType::kUnknown;
      }
    }
  }

  template <class LabelMatcher>
  QuerierStatus query_values_by_regexp(const LabelMatcher& label_matcher, const TrieIndex::Trie* trie, Selector::Matcher& matcher) {
    auto regexp = RegexpParser::parse(static_cast<std::string_view>(label_matcher.value));
    switch (RegexpMatchAnalyzer::analyze(regexp.get())) {
      case RegexpMatchAnalyzer::Status::kError: {
        matcher.status = MatchStatus::kError;
        return QuerierStatus::kRegexpError;
      }

      case RegexpMatchAnalyzer::Status::kAllMatch: {
        if (trie == nullptr) {
          matcher.status = MatchStatus::kEmptyMatch;
          return QuerierStatus::kNoMatch;
        }

        matcher.status = MatchStatus::kAllMatch;
        return QuerierStatus::kMatch;
      }

      case RegexpMatchAnalyzer::Status::kPartialMatch: {
        if (trie == nullptr) {
          matcher.status = MatchStatus::kEmptyMatch;
          return QuerierStatus::kNoMatch;
        }

        typename TrieIndex::RegexpMatchesList matches_list(matcher.matches);
        if (auto status = RegexpSearcher<typename TrieIndex::Trie, typename TrieIndex::RegexpMatchesList>(matches_list).search(*trie, regexp);
            status == MatchStatus::kEmptyMatch) {
          matcher.status = MatchStatus::kEmptyMatch;
          return QuerierStatus::kNoMatch;
        }

        matcher.status = MatchStatus::kPartialMatch;
        return QuerierStatus::kMatch;
      }

      case RegexpMatchAnalyzer::Status::kEmptyMatch: {
        process_empty_matcher(matcher, trie);
        return QuerierStatus::kMatch;
      }

      case RegexpMatchAnalyzer::Status::kAnythingMatch: {
        matcher.status = MatchStatus::kAllMatch;
        matcher.type = MatcherType::kUnknown;
        return QuerierStatus::kMatch;
      }

      default: {
        assert(false);
        return QuerierStatus::kRegexpError;
      }
    }
  }
};

}  // namespace series_index::querier