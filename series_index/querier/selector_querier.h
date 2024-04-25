#pragma once

#include <cassert>

#include "prometheus/selector.h"
#include "regexp_searcher.h"
#include "series_index/trie_index.h"

namespace series_index::querier {

enum class QuerierStatus {
  kNoPositiveMatchers = 0,
  kRegexpError,
  kNoMatch,
  kMatch,
};

template <class TrieIndex>
class SelectorQuerier {
 public:
  using LabelMatcher = PromPP::Prometheus::LabelMatcher;
  using Selector = PromPP::Prometheus::Selector;
  using MatchStatus = PromPP::Prometheus::MatchStatus;

  explicit SelectorQuerier(const TrieIndex& index) : index_(index) {}

  [[nodiscard]] QuerierStatus query(Selector& selector) {
    for (auto& matcher : selector.matchers) {
      if (matcher.matcher.is_positive()) {
        if (auto status = query(matcher); status != QuerierStatus::kMatch) {
          return status;
        }
      }
    }

    if (!selector.have_positive_matchers()) {
      return QuerierStatus::kNoPositiveMatchers;
    }

    for (auto& matcher : selector.matchers) {
      if (matcher.matcher.is_negative() && matcher.result.status == MatchStatus::kUnknown) {
        if (auto status = query(matcher); status != QuerierStatus::kMatch && status != QuerierStatus::kNoMatch) {
          return status;
        }

        if (matcher.matcher.is_unknown()) {
          return QuerierStatus::kNoMatch;
        }
      }
    }

    return QuerierStatus::kMatch;
  }

  PROMPP_ALWAYS_INLINE QuerierStatus query(Selector::Matcher& matcher) { return query_values(matcher, get_values_trie(matcher)); }

 private:
  const TrieIndex& index_;

  PROMPP_ALWAYS_INLINE const TrieIndex::Trie* get_values_trie(Selector::Matcher& matcher) const noexcept {
    if (auto index = index_.names_trie().lookup(matcher.matcher.name); index) {
      matcher.result.label_name_id = *index;
      return index_.values_trie(*index);
    }

    return nullptr;
  }

  QuerierStatus query_values(Selector::Matcher& matcher, const TrieIndex::Trie* trie) {
    if (matcher.matcher.value.empty()) {
      process_empty_matcher(matcher, trie);
      return QuerierStatus::kMatch;
    }

    switch (matcher.matcher.type) {
      case LabelMatcher::Type::kExactMatch:
      case LabelMatcher::Type::kExactNotMatch: {
        return query_exact_value(matcher, trie);
      }

      case LabelMatcher::Type::kRegexpMatch:
      case LabelMatcher::Type::kRegexpNotMatch: {
        return query_values_by_regexp(matcher, trie);
      }

      default: {
        assert(false);
        return QuerierStatus::kNoMatch;
      }
    }
  }

  QuerierStatus query_exact_value(Selector::Matcher& matcher, const TrieIndex::Trie* trie) {
    if (trie == nullptr) {
      matcher.result.status = MatchStatus::kEmptyMatch;
      return QuerierStatus::kNoMatch;
    }

    if (auto value = trie->lookup(matcher.matcher.value); value) {
      matcher.result.matches.emplace_back(*value);
      matcher.result.status = MatchStatus::kPartialMatch;
      return QuerierStatus::kMatch;
    }

    matcher.result.status = MatchStatus::kEmptyMatch;
    return QuerierStatus::kNoMatch;
  }

  void process_empty_matcher(Selector::Matcher& matcher, const TrieIndex::Trie* trie) {
    if (matcher.matcher.is_positive()) {
      matcher.matcher.convert_to_negative();

      if (trie != nullptr) {
        matcher.result.status = MatchStatus::kAllMatch;
      } else {
        matcher.result.status = MatchStatus::kEmptyMatch;
      }
    } else {
      if (trie != nullptr) {
        matcher.result.status = MatchStatus::kAllMatch;
      } else {
        matcher.result.status = MatchStatus::kEmptyMatch;
        matcher.matcher.invalidate();
      }
    }
  }

  QuerierStatus query_values_by_regexp(Selector::Matcher& matcher, const TrieIndex::Trie* trie) {
    auto regexp = RegexpParser::parse(matcher.matcher.value);
    switch (RegexpMatchAnalyzer::analyze(regexp.get())) {
      case RegexpMatchAnalyzer::Status::kError: {
        matcher.result.status = MatchStatus::kError;
        return QuerierStatus::kRegexpError;
      }

      case RegexpMatchAnalyzer::Status::kAllMatch: {
        if (trie == nullptr) {
          matcher.result.status = MatchStatus::kEmptyMatch;
          return QuerierStatus::kNoMatch;
        }

        matcher.result.status = MatchStatus::kAllMatch;
        return QuerierStatus::kMatch;
      }

      case RegexpMatchAnalyzer::Status::kPartialMatch: {
        if (trie == nullptr) {
          matcher.result.status = MatchStatus::kEmptyMatch;
          return QuerierStatus::kNoMatch;
        }

        typename TrieIndex::RegexpMatchesList matches_list(matcher.result.matches);
        if (auto status = RegexpSearcher<typename TrieIndex::Trie, typename TrieIndex::RegexpMatchesList>(matches_list).search(*trie, regexp);
            status == MatchStatus::kEmptyMatch) {
          matcher.result.status = MatchStatus::kEmptyMatch;
          return QuerierStatus::kNoMatch;
        }

        matcher.result.status = MatchStatus::kPartialMatch;
        return QuerierStatus::kMatch;
      }

      case RegexpMatchAnalyzer::Status::kEmptyMatch: {
        process_empty_matcher(matcher, trie);
        return QuerierStatus::kMatch;
      }

      case RegexpMatchAnalyzer::Status::kAnythingMatch: {
        matcher.result.status = MatchStatus::kAllMatch;
        matcher.matcher.invalidate();
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