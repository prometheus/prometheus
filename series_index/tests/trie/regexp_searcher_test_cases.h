#pragma once

#include <gtest/gtest.h>

namespace series_index::querier::regexp_tests {

using std::operator""sv;

struct RegexpSearcherTestCase {
  std::vector<std::string_view> trie_values;
  std::string_view regexp;
  std::vector<std::string_view> matches;
};

static inline const auto kSearchByPrefixCases =
    testing::Values(RegexpSearcherTestCase{.trie_values = {"abc", "abd"}, .regexp = "abcd", .matches = {}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "abd"}, .regexp = "ac", .matches = {}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "cba"}, .regexp = "abc", .matches = {"abc"}},
                    RegexpSearcherTestCase{.trie_values = {"a\000c"sv, "a\000d"sv}, .regexp = R"(a\x00c)", .matches = {"a\000c"sv}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "abcd"}, .regexp = "abc(bc|cd){0}", .matches = {"abc"}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "cba"}, .regexp = "abc(bc|cd){0}", .matches = {"abc"}},
                    RegexpSearcherTestCase{.trie_values = {"a\000c"sv, "a\000d"sv}, .regexp = R"(a\x00(c|d))", .matches = {"a\000c"sv, "a\000d"sv}},
                    RegexpSearcherTestCase{.trie_values = {"a\000c"sv, "a\000d"sv}, .regexp = R"(a\x00c(b|d){0})", .matches = {"a\000c"sv}},
                    RegexpSearcherTestCase{.trie_values = {"abcd", "abcd-1", "abcd-2"}, .regexp = "abcd", .matches = {"abcd"}},
                    RegexpSearcherTestCase{.trie_values = {"abcde-1", "abcde-2"}, .regexp = "abcd", .matches = {}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "acd"}, .regexp = "a", .matches = {}});

static inline const auto kSearchAlternativesCases =
    testing::Values(RegexpSearcherTestCase{.trie_values = {"abc", "cba", "cbb"}, .regexp = "abc|cba", .matches = {"abc", "cba"}},
                    RegexpSearcherTestCase{.trie_values = {"\x00"sv, "\x00\x00"sv}, .regexp = R"((\x00|\x00\x00))", .matches = {"\x00"sv, "\x00\x00"sv}});

static inline const auto kSearchCharacterClass =
    testing::Values(RegexpSearcherTestCase{.trie_values = {"abc", "abd", "abb"}, .regexp = "ab[cd]", .matches = {"abc", "abd"}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "acd"}, .regexp = "ab[cd]", .matches = {"abc"}},
                    RegexpSearcherTestCase{.trie_values = {"ab\x00"sv, "acd"}, .regexp = "ab[\000d]"sv, .matches = {"ab\x00"sv}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "abcd"}, .regexp = "ab[cd]$", .matches = {"abc"}},
                    RegexpSearcherTestCase{.trie_values = {"ab\x00"sv, "abcd"}, .regexp = "ab[\000d]$"sv, .matches = {"ab\x00"sv}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "acd"}, .regexp = "ad[cd]", .matches = {}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "acd"}, .regexp = "a[^0-9]", .matches = {}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "acd", "a\x00"sv}, .regexp = "a[^0-9].*", .matches = {"abc", "acd", "a\x00"sv}},
                    RegexpSearcherTestCase{.trie_values = {"a_bc", "a_bc_", "a_bc_0", "a_bc_1", "a_cd", "a_cd_", "a_cd_0"},
                                           .regexp = "a_(bc|cd)_.*",
                                           .matches = {"a_bc_", "a_bc_0", "a_bc_1", "a_cd_", "a_cd_0"}});

static inline const auto kSearchByPrefixAndRegexpCases =
    testing::Values(RegexpSearcherTestCase{.trie_values = {"abc"}, .regexp = "ab.*", .matches = {"abc"}},
                    RegexpSearcherTestCase{.trie_values = {"ab", "abc", "ab\x00"sv}, .regexp = "ab.*", .matches = {"ab", "abc", "ab\x00"sv}},
                    RegexpSearcherTestCase{.trie_values = {"ab"}, .regexp = "ab.*", .matches = {"ab"}},
                    RegexpSearcherTestCase{.trie_values = {"ab", "abc", "ab\x00"sv}, .regexp = "ab.+", .matches = {"abc", "ab\x00"sv}},
                    RegexpSearcherTestCase{.trie_values = {"ab", "abc"}, .regexp = "abc+", .matches = {"abc"}},
                    RegexpSearcherTestCase{.trie_values = {"ab", "abc"}, .regexp = "abc*", .matches = {"ab", "abc"}},
                    RegexpSearcherTestCase{.trie_values = {"ab"}, .regexp = "ab.+", .matches = {}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "abd", "abb"}, .regexp = "ab.*", .matches = {"abc", "abd", "abb"}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "abd", "abb"}, .regexp = "abc.*", .matches = {"abc"}},
                    RegexpSearcherTestCase{.trie_values = {"ab"}, .regexp = "abc?", .matches = {"ab"}},
                    RegexpSearcherTestCase{.trie_values = {"abcde", "abcfg"}, .regexp = "abc$", .matches = {}},
                    RegexpSearcherTestCase{.trie_values = {"abcde"}, .regexp = "a.+", .matches = {"abcde"}});

static inline const auto kSearchByRegexpCases = testing::Values(RegexpSearcherTestCase{.trie_values = {"abc"}, .regexp = "^.{5,}", .matches = {}});

static inline const auto kSearchByRegexpWithEndTextCases = testing::Values(RegexpSearcherTestCase{.trie_values = {"abc"}, .regexp = ".{5,}$", .matches = {}});

static inline const auto kSearchByCaseInsensitiveCases =
    testing::Values(RegexpSearcherTestCase{.trie_values = {"abCdE"}, .regexp = "(?i)ABcDe", .matches = {"abCdE"}},
                    RegexpSearcherTestCase{.trie_values = {"abCdE"}, .regexp = "(?i)ABcDe.*", .matches = {"abCdE"}},
                    RegexpSearcherTestCase{.trie_values = {"abC\000dE\000"sv}, .regexp = "(?i)ABc.De.*", .matches = {"abC\000dE\000"sv}});

#define INSTANTIATE_REGEXP_SEARCHER_TEST_SUITE_P(trie_fixture)                                                                             \
  INSTANTIATE_TEST_SUITE_P(SearchByPrefix, trie_fixture, series_index::querier::regexp_tests::kSearchByPrefixCases);                       \
  INSTANTIATE_TEST_SUITE_P(SearchAlternatives, trie_fixture, series_index::querier::regexp_tests::kSearchAlternativesCases);               \
  INSTANTIATE_TEST_SUITE_P(SearchCharacterClass, trie_fixture, series_index::querier::regexp_tests::kSearchCharacterClass);                \
  INSTANTIATE_TEST_SUITE_P(SearchByPrefixAndRegexp, trie_fixture, series_index::querier::regexp_tests::kSearchByPrefixAndRegexpCases);     \
  INSTANTIATE_TEST_SUITE_P(SearchByRegexp, trie_fixture, series_index::querier::regexp_tests::kSearchByRegexpCases);                       \
  INSTANTIATE_TEST_SUITE_P(SearchByRegexpWithEndText, trie_fixture, series_index::querier::regexp_tests::kSearchByRegexpWithEndTextCases); \
  INSTANTIATE_TEST_SUITE_P(SearchByCaseInsensitive, trie_fixture, series_index::querier::regexp_tests::kSearchByCaseInsensitiveCases)

}  // namespace series_index::querier::regexp_tests