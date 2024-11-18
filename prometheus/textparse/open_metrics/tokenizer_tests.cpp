#include <gtest/gtest.h>

#include <vector>

#include "tokenizer.h"

namespace {

using PromPP::Prometheus::textparse::OpenMetrics::Tokenizer;
using std::operator""sv;

using TokenType = PromPP::Prometheus::textparse::Token;

struct Token {
  std::string_view text;
  TokenType token;

  bool operator==(const Token& other) const noexcept = default;
};

std::ostream& operator<<(std::ostream& stream, const Token& token) {
  stream << "[ type: " << static_cast<int>(token.token) << ", token: `" << token.text << "` ]";
  return stream;
}

struct TokenizerCase {
  std::string_view str;
  std::vector<Token> tokens;
};

class OpenMetricsTokenizerFixture : public ::testing::TestWithParam<TokenizerCase> {
 protected:
  std::string shrinked_str_;

  std::vector<Token> tokenize(std::string_view str) noexcept {
    std::vector<Token> tokens;

    shrinked_str_.assign(str);
    shrinked_str_.shrink_to_fit();

    Tokenizer tokenizer(shrinked_str_);
    while (tokenizer.next() != TokenType::kEOF) {
      tokens.emplace_back(Token{.text = tokenizer.token_str(), .token = tokenizer.token()});
    }

    return tokens;
  }
};

TEST_P(OpenMetricsTokenizerFixture, Test) {
  // Arrange

  // Act
  const auto tokens = tokenize(GetParam().str);

  // Assert
  EXPECT_EQ(GetParam().tokens, tokens);
}

INSTANTIATE_TEST_SUITE_P(EmptyString,
                         OpenMetricsTokenizerFixture,
                         testing::Values(TokenizerCase{.str = "", .tokens = {}}, TokenizerCase{.str = "\x00"sv, .tokens = {}}));
INSTANTIATE_TEST_SUITE_P(LineBreak,
                         OpenMetricsTokenizerFixture,
                         testing::Values(TokenizerCase{.str = "\n", .tokens = {Token{.text = "\n", .token = TokenType::kLinebreak}}}));
INSTANTIATE_TEST_SUITE_P(HelpMeta,
                         OpenMetricsTokenizerFixture,
                         testing::Values(TokenizerCase{.str = "# HELP go_gc_duration_seconds A summary of the pause duration of garbage collection cycles.\n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "HELP ", .token = TokenType::kHelp},
                                                               Token{.text = "go_gc_duration_seconds", .token = TokenType::kMetricName},
                                                               Token{.text = "A summary of the pause duration of garbage collection cycles.",
                                                                     .token = TokenType::kText},
                                                               Token{.text = "\n", .token = TokenType::kLinebreak},
                                                           }},
                                         TokenizerCase{.str = "# HELP go_gc_duration_seconds  \n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "HELP ", .token = TokenType::kHelp},
                                                               Token{.text = "go_gc_duration_seconds", .token = TokenType::kMetricName},
                                                               Token{.text = "", .token = TokenType::kText},
                                                               Token{.text = "\n", .token = TokenType::kLinebreak},
                                                           }},
                                         TokenizerCase{.str = R"(# HELP "go_gc_duration_seconds" Some text and \n some \" escaping)"
                                                              "\n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "HELP ", .token = TokenType::kHelp},
                                                               Token{.text = R"("go_gc_duration_seconds")", .token = TokenType::kMetricName},
                                                               Token{.text = R"(Some text and \n some \" escaping)", .token = TokenType::kText},
                                                               Token{.text = "\n", .token = TokenType::kLinebreak},
                                                           }},
                                         TokenizerCase{.str = "# HELP metric foo\000bar\n"sv,
                                                       .tokens = {
                                                           Token{.text = "HELP ", .token = TokenType::kHelp},
                                                           Token{.text = "metric", .token = TokenType::kMetricName},
                                                           Token{.text = "foo\000bar"sv, .token = TokenType::kText},
                                                           Token{.text = "\n", .token = TokenType::kLinebreak},
                                                       }}));
INSTANTIATE_TEST_SUITE_P(TypeMeta,
                         OpenMetricsTokenizerFixture,
                         testing::Values(TokenizerCase{.str = "# TYPE go_gc_duration_seconds summary\n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "TYPE ", .token = TokenType::kType},
                                                               Token{.text = "go_gc_duration_seconds", .token = TokenType::kMetricName},
                                                               Token{.text = "summary", .token = TokenType::kText},
                                                               Token{.text = "\n", .token = TokenType::kLinebreak},
                                                           }},
                                         TokenizerCase{.str = "# TYPE go_gc_duration_seconds  \n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "TYPE ", .token = TokenType::kType},
                                                               Token{.text = "go_gc_duration_seconds", .token = TokenType::kMetricName},
                                                               Token{.text = "", .token = TokenType::kText},
                                                               Token{.text = "\n", .token = TokenType::kLinebreak},
                                                           }},
                                         TokenizerCase{.str = R"(# TYPE "go_gc_duration_seconds" Some text and \n some \" escaping)"
                                                              "\n",
                                                       .tokens = {
                                                           Token{.text = "TYPE ", .token = TokenType::kType},
                                                           Token{.text = R"("go_gc_duration_seconds")", .token = TokenType::kMetricName},
                                                           Token{.text = R"(Some text and \n some \" escaping)", .token = TokenType::kText},
                                                           Token{.text = "\n", .token = TokenType::kLinebreak},
                                                       }}));
INSTANTIATE_TEST_SUITE_P(UnitMeta,
                         OpenMetricsTokenizerFixture,
                         testing::Values(TokenizerCase{.str = "# UNIT go_gc_duration_seconds seconds\n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "UNIT ", .token = TokenType::kUnit},
                                                               Token{.text = "go_gc_duration_seconds", .token = TokenType::kMetricName},
                                                               Token{.text = "seconds", .token = TokenType::kText},
                                                               Token{.text = "\n", .token = TokenType::kLinebreak},
                                                           }},
                                         TokenizerCase{.str = "# UNIT go_gc_duration_seconds  \n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "UNIT ", .token = TokenType::kUnit},
                                                               Token{.text = "go_gc_duration_seconds", .token = TokenType::kMetricName},
                                                               Token{.text = "", .token = TokenType::kText},
                                                               Token{.text = "\n", .token = TokenType::kLinebreak},
                                                           }},
                                         TokenizerCase{.str = R"(# UNIT "go_gc_duration_seconds" Some text and \n some \" escaping)"
                                                              "\n",
                                                       .tokens = {
                                                           Token{.text = "UNIT ", .token = TokenType::kUnit},
                                                           Token{.text = R"("go_gc_duration_seconds")", .token = TokenType::kMetricName},
                                                           Token{.text = R"(Some text and \n some \" escaping)", .token = TokenType::kText},
                                                           Token{.text = "\n", .token = TokenType::kLinebreak},
                                                       }}));
INSTANTIATE_TEST_SUITE_P(EOFWord,
                         OpenMetricsTokenizerFixture,
                         testing::Values(TokenizerCase{.str = "# EOF\n", .tokens = {Token{.text = "EOF\n", .token = TokenType::kEOFWord}}},
                                         TokenizerCase{.str = "# EOF", .tokens = {Token{.text = "EOF", .token = TokenType::kEOFWord}}}));

INSTANTIATE_TEST_SUITE_P(Label,
                         OpenMetricsTokenizerFixture,
                         testing::Values(TokenizerCase{.str = "go_gc_duration_seconds{quantile=\"0\"} 4.8099e-05\n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "go_gc_duration_seconds", .token = TokenType::kMetricName},
                                                               Token{.text = "{", .token = TokenType::kBraceOpen},
                                                               Token{.text = "quantile", .token = TokenType::kLabelName},
                                                               Token{.text = "=", .token = TokenType::kEqual},
                                                               Token{.text = R"("0")", .token = TokenType::kLabelValue},
                                                               Token{.text = "}", .token = TokenType::kBraceClose},
                                                               Token{.text = "4.8099e-05", .token = TokenType::kValue},
                                                               Token{.text = "\n", .token = TokenType::kLinebreak},
                                                           }},
                                         TokenizerCase{.str = "go_gc_duration_seconds_count 8437\n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "go_gc_duration_seconds_count", .token = TokenType::kMetricName},
                                                               Token{.text = "8437", .token = TokenType::kValue},
                                                               Token{.text = "\n", .token = TokenType::kLinebreak},
                                                           }},
                                         TokenizerCase{.str = R"(bar_seconds_count{a="x",b="escaping\" example \n "} 0)"
                                                              "\n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "bar_seconds_count", .token = TokenType::kMetricName},
                                                               Token{.text = "{", .token = TokenType::kBraceOpen},
                                                               Token{.text = "a", .token = TokenType::kLabelName},
                                                               Token{.text = "=", .token = TokenType::kEqual},
                                                               Token{.text = R"("x")", .token = TokenType::kLabelValue},
                                                               Token{.text = ",", .token = TokenType::kComma},
                                                               Token{.text = "b", .token = TokenType::kLabelName},
                                                               Token{.text = "=", .token = TokenType::kEqual},
                                                               Token{.text = R"("escaping\" example \n ")", .token = TokenType::kLabelValue},
                                                               Token{.text = "}", .token = TokenType::kBraceClose},
                                                               Token{.text = "0", .token = TokenType::kValue},
                                                               Token{.text = "\n", .token = TokenType::kLinebreak},
                                                           }},
                                         TokenizerCase{.str = "bar_seconds_count{} 0\n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "bar_seconds_count", .token = TokenType::kMetricName},
                                                               Token{.text = "{", .token = TokenType::kBraceOpen},
                                                               Token{.text = "}", .token = TokenType::kBraceClose},
                                                               Token{.text = "0", .token = TokenType::kValue},
                                                               Token{.text = "\n", .token = TokenType::kLinebreak},
                                                           }},
                                         TokenizerCase{.str = "foo_seconds_count{a=\"bb\"} 0 123.123\n",
                                                       .tokens = {
                                                           Token{.text = "foo_seconds_count", .token = TokenType::kMetricName},
                                                           Token{.text = "{", .token = TokenType::kBraceOpen},
                                                           Token{.text = "a", .token = TokenType::kLabelName},
                                                           Token{.text = "=", .token = TokenType::kEqual},
                                                           Token{.text = R"("bb")", .token = TokenType::kLabelValue},
                                                           Token{.text = "}", .token = TokenType::kBraceClose},
                                                           Token{.text = "0", .token = TokenType::kValue},
                                                           Token{.text = "123.123", .token = TokenType::kTimestamp},
                                                           Token{.text = "\n", .token = TokenType::kLinebreak},
                                                       }}));

INSTANTIATE_TEST_SUITE_P(Exemplars,
                         OpenMetricsTokenizerFixture,
                         testing::Values(TokenizerCase{.str = "hhh_count 1 # {id=\"histogram-count-test\"} 4\n# EOF",
                                                       .tokens =
                                                           {
                                                               Token{.text = "hhh_count", .token = TokenType::kMetricName},
                                                               Token{.text = "1", .token = TokenType::kValue},
                                                               Token{.text = " # {", .token = TokenType::kExemplar},
                                                               Token{.text = "id", .token = TokenType::kLabelName},
                                                               Token{.text = "=", .token = TokenType::kEqual},
                                                               Token{.text = "\"histogram-count-test\"", .token = TokenType::kLabelValue},
                                                               Token{.text = "}", .token = TokenType::kBraceClose},
                                                               Token{.text = "4", .token = TokenType::kValue},
                                                               Token{.text = "\n", .token = TokenType::kLinebreak},
                                                               Token{.text = "EOF", .token = TokenType::kEOFWord},
                                                           }},
                                         TokenizerCase{.str = "hhh_count 1 # {id=\"histogram-count-test\"} 4 123.123\n# EOF",
                                                       .tokens =
                                                           {
                                                               Token{.text = "hhh_count", .token = TokenType::kMetricName},
                                                               Token{.text = "1", .token = TokenType::kValue},
                                                               Token{.text = " # {", .token = TokenType::kExemplar},
                                                               Token{.text = "id", .token = TokenType::kLabelName},
                                                               Token{.text = "=", .token = TokenType::kEqual},
                                                               Token{.text = "\"histogram-count-test\"", .token = TokenType::kLabelValue},
                                                               Token{.text = "}", .token = TokenType::kBraceClose},
                                                               Token{.text = "4", .token = TokenType::kValue},
                                                               Token{.text = "123.123", .token = TokenType::kTimestamp},
                                                               Token{.text = "\n", .token = TokenType::kLinebreak},
                                                               Token{.text = "EOF", .token = TokenType::kEOFWord},
                                                           }},
                                         TokenizerCase{.str = "hhh_count 1 # {} 4 123.123\n# EOF",
                                                       .tokens =
                                                           {
                                                               Token{.text = "hhh_count", .token = TokenType::kMetricName},
                                                               Token{.text = "1", .token = TokenType::kValue},
                                                               Token{.text = " # {", .token = TokenType::kExemplar},
                                                               Token{.text = "}", .token = TokenType::kBraceClose},
                                                               Token{.text = "4", .token = TokenType::kValue},
                                                               Token{.text = "123.123", .token = TokenType::kTimestamp},
                                                               Token{.text = "\n", .token = TokenType::kLinebreak},
                                                               Token{.text = "EOF", .token = TokenType::kEOFWord},
                                                           }},
                                         TokenizerCase{.str = "hhh_count 1 123.123 # {} 4 123.123\n# EOF",
                                                       .tokens =
                                                           {
                                                               Token{.text = "hhh_count", .token = TokenType::kMetricName},
                                                               Token{.text = "1", .token = TokenType::kValue},
                                                               Token{.text = "123.123", .token = TokenType::kTimestamp},
                                                               Token{.text = " # {", .token = TokenType::kExemplar},
                                                               Token{.text = "}", .token = TokenType::kBraceClose},
                                                               Token{.text = "4", .token = TokenType::kValue},
                                                               Token{.text = "123.123", .token = TokenType::kTimestamp},
                                                               Token{.text = "\n", .token = TokenType::kLinebreak},
                                                               Token{.text = "EOF", .token = TokenType::kEOFWord},
                                                           }}/*,
                                         TokenizerCase{.str = "hhh_count 1 123.123 # {} 4 123.123 # {} 4 123.123\n# EOF",
                                                       .tokens = {
                                                           Token{.text = "hhh_count", .token = TokenType::kMetricName},
                                                           Token{.text = "1", .token = TokenType::kValue},
                                                           Token{.text = "123.123", .token = TokenType::kTimestamp},
                                                           Token{.text = "# {", .token = TokenType::kComment},
                                                           Token{.text = "}", .token = TokenType::kBraceClose},
                                                           Token{.text = "4", .token = TokenType::kExemplarValue},
                                                           Token{.text = "123.123", .token = TokenType::kExemplarTimestamp},
                                                           Token{.text = "# {} 4 123.123\n", .token = TokenType::kInvalid},
                                                       }}*/));

}  // namespace