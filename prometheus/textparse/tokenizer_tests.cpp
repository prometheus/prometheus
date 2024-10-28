#include <gtest/gtest.h>

#include <vector>

#include "tokenizer.h"

namespace {

using PromPP::Prometheus::textparse::Tokenizer;
using std::operator""sv;

struct Token {
  std::string_view text;
  Tokenizer::Token token;

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

class TokenizerFixture : public ::testing::TestWithParam<TokenizerCase> {
 protected:
  static std::vector<Token> tokenize(std::string_view str) noexcept {
    std::vector<Token> tokens;

    Tokenizer tokenizer(str);
    while (tokenizer.next() != Tokenizer::Token::kEOF) {
      tokens.emplace_back(Token{.text = tokenizer.token_str(), .token = tokenizer.token()});
    }

    return tokens;
  }
};

TEST_P(TokenizerFixture, Test) {
  // Arrange

  // Act
  const auto tokens = tokenize(GetParam().str);

  // Assert
  EXPECT_EQ(GetParam().tokens, tokens);
}

INSTANTIATE_TEST_SUITE_P(EmptyString, TokenizerFixture, testing::Values(TokenizerCase{.str = "", .tokens = {}}, TokenizerCase{.str = "\x00"sv, .tokens = {}}));
INSTANTIATE_TEST_SUITE_P(Comment,
                         TokenizerFixture,
                         testing::Values(TokenizerCase{.str = "#HELP\n", .tokens = {Token{.text = "HELP", .token = Tokenizer::Token::kComment}}},
                                         TokenizerCase{.str = "# ABCD\n", .tokens = {Token{.text = "ABCD", .token = Tokenizer::Token::kComment}}},
                                         TokenizerCase{.str = "# HELPing\n", .tokens = {Token{.text = "HELPing", .token = Tokenizer::Token::kComment}}},
                                         TokenizerCase{.str = "#\n", .tokens = {Token{.text = "", .token = Tokenizer::Token::kComment}}},
                                         TokenizerCase{.str = "#\n\n#\n",
                                                       .tokens = {
                                                           Token{.text = "", .token = Tokenizer::Token::kComment},
                                                           Token{.text = "\n", .token = Tokenizer::Token::kLinebreak},
                                                           Token{.text = "\n", .token = Tokenizer::Token::kLinebreak},
                                                           Token{.text = "", .token = Tokenizer::Token::kComment},
                                                       }}));
INSTANTIATE_TEST_SUITE_P(HelpMeta,
                         TokenizerFixture,
                         testing::Values(TokenizerCase{.str = "# HELP go_gc_duration_seconds A summary of the pause duration of garbage collection cycles.\n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "HELP ", .token = Tokenizer::Token::kHelp},
                                                               Token{.text = "go_gc_duration_seconds", .token = Tokenizer::Token::kMetricName},
                                                               Token{.text = "A summary of the pause duration of garbage collection cycles.",
                                                                     .token = Tokenizer::Token::kText},
                                                           }},
                                         TokenizerCase{.str = "# HELP go_gc_duration_seconds\n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "HELP ", .token = Tokenizer::Token::kHelp},
                                                               Token{.text = "go_gc_duration_seconds", .token = Tokenizer::Token::kMetricName},
                                                               Token{.text = "", .token = Tokenizer::Token::kText},
                                                           }},
                                         TokenizerCase{.str = "# HELP go_gc_duration_seconds  \n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "HELP ", .token = Tokenizer::Token::kHelp},
                                                               Token{.text = "go_gc_duration_seconds", .token = Tokenizer::Token::kMetricName},
                                                               Token{.text = "", .token = Tokenizer::Token::kText},
                                                           }},
                                         TokenizerCase{.str = R"(# HELP "go_gc_duration_seconds" Some text and \n some \" escaping)"
                                                              "\n",
                                                       .tokens = {
                                                           Token{.text = "HELP ", .token = Tokenizer::Token::kHelp},
                                                           Token{.text = R"("go_gc_duration_seconds")", .token = Tokenizer::Token::kMetricName},
                                                           Token{.text = R"(Some text and \n some \" escaping)", .token = Tokenizer::Token::kText},
                                                       }}));
INSTANTIATE_TEST_SUITE_P(TypeMeta,
                         TokenizerFixture,
                         testing::Values(TokenizerCase{.str = "# TYPE go_gc_duration_seconds summary\n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "TYPE ", .token = Tokenizer::Token::kType},
                                                               Token{.text = "go_gc_duration_seconds", .token = Tokenizer::Token::kMetricName},
                                                               Token{.text = "summary", .token = Tokenizer::Token::kText},
                                                           }},
                                         TokenizerCase{.str = "# TYPE go_gc_duration_seconds\n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "TYPE ", .token = Tokenizer::Token::kType},
                                                               Token{.text = "go_gc_duration_seconds", .token = Tokenizer::Token::kMetricName},
                                                               Token{.text = "", .token = Tokenizer::Token::kText},
                                                           }},
                                         TokenizerCase{.str = "# TYPE go_gc_duration_seconds  \n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "TYPE ", .token = Tokenizer::Token::kType},
                                                               Token{.text = "go_gc_duration_seconds", .token = Tokenizer::Token::kMetricName},
                                                               Token{.text = "", .token = Tokenizer::Token::kText},
                                                           }},
                                         TokenizerCase{.str = R"(# TYPE "go_gc_duration_seconds" Some text and \n some \" escaping)"
                                                              "\n",
                                                       .tokens = {
                                                           Token{.text = "TYPE ", .token = Tokenizer::Token::kType},
                                                           Token{.text = R"("go_gc_duration_seconds")", .token = Tokenizer::Token::kMetricName},
                                                           Token{.text = R"(Some text and \n some \" escaping)", .token = Tokenizer::Token::kText},
                                                       }}));

INSTANTIATE_TEST_SUITE_P(Label,
                         TokenizerFixture,
                         testing::Values(TokenizerCase{.str = "go_gc_duration_seconds{quantile=\"0\"} 4.8099e-05\n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "go_gc_duration_seconds", .token = Tokenizer::Token::kMetricName},
                                                               Token{.text = "{", .token = Tokenizer::Token::kBraceOpen},
                                                               Token{.text = "quantile", .token = Tokenizer::Token::kLabelName},
                                                               Token{.text = "=", .token = Tokenizer::Token::kEqual},
                                                               Token{.text = R"("0")", .token = Tokenizer::Token::kLabelValue},
                                                               Token{.text = "}", .token = Tokenizer::Token::kBraceClose},
                                                               Token{.text = " ", .token = Tokenizer::Token::kWhitespace},
                                                               Token{.text = "4.8099e-05", .token = Tokenizer::Token::kValue},
                                                               Token{.text = "\n", .token = Tokenizer::Token::kLinebreak},
                                                           }},
                                         TokenizerCase{.str = "go_gc_duration_seconds_count 8437\n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "go_gc_duration_seconds_count", .token = Tokenizer::Token::kMetricName},
                                                               Token{.text = " ", .token = Tokenizer::Token::kWhitespace},
                                                               Token{.text = "8437", .token = Tokenizer::Token::kValue},
                                                               Token{.text = "\n", .token = Tokenizer::Token::kLinebreak},
                                                           }},
                                         TokenizerCase{.str = R"(bar_seconds_count{a="x",b="escaping\" example \n "} 0)"
                                                              "\n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "bar_seconds_count", .token = Tokenizer::Token::kMetricName},
                                                               Token{.text = "{", .token = Tokenizer::Token::kBraceOpen},
                                                               Token{.text = "a", .token = Tokenizer::Token::kLabelName},
                                                               Token{.text = "=", .token = Tokenizer::Token::kEqual},
                                                               Token{.text = R"("x")", .token = Tokenizer::Token::kLabelValue},
                                                               Token{.text = ",", .token = Tokenizer::Token::kComma},
                                                               Token{.text = "b", .token = Tokenizer::Token::kLabelName},
                                                               Token{.text = "=", .token = Tokenizer::Token::kEqual},
                                                               Token{.text = R"("escaping\" example \n ")", .token = Tokenizer::Token::kLabelValue},
                                                               Token{.text = "}", .token = Tokenizer::Token::kBraceClose},
                                                               Token{.text = " ", .token = Tokenizer::Token::kWhitespace},
                                                               Token{.text = "0", .token = Tokenizer::Token::kValue},
                                                               Token{.text = "\n", .token = Tokenizer::Token::kLinebreak},
                                                           }},
                                         TokenizerCase{.str = "bar_seconds_count{} 0\n",
                                                       .tokens =
                                                           {
                                                               Token{.text = "bar_seconds_count", .token = Tokenizer::Token::kMetricName},
                                                               Token{.text = "{", .token = Tokenizer::Token::kBraceOpen},
                                                               Token{.text = "}", .token = Tokenizer::Token::kBraceClose},
                                                               Token{.text = " ", .token = Tokenizer::Token::kWhitespace},
                                                               Token{.text = "0", .token = Tokenizer::Token::kValue},
                                                               Token{.text = "\n", .token = Tokenizer::Token::kLinebreak},
                                                           }},
                                         TokenizerCase{.str = "foo_seconds_count{a=\"bb\"} 0 123\n",
                                                       .tokens = {
                                                           Token{.text = "foo_seconds_count", .token = Tokenizer::Token::kMetricName},
                                                           Token{.text = "{", .token = Tokenizer::Token::kBraceOpen},
                                                           Token{.text = "a", .token = Tokenizer::Token::kLabelName},
                                                           Token{.text = "=", .token = Tokenizer::Token::kEqual},
                                                           Token{.text = R"("bb")", .token = Tokenizer::Token::kLabelValue},
                                                           Token{.text = "}", .token = Tokenizer::Token::kBraceClose},
                                                           Token{.text = " ", .token = Tokenizer::Token::kWhitespace},
                                                           Token{.text = "0", .token = Tokenizer::Token::kValue},
                                                           Token{.text = " ", .token = Tokenizer::Token::kWhitespace},
                                                           Token{.text = "123", .token = Tokenizer::Token::kTimestamp},
                                                           Token{.text = "\n", .token = Tokenizer::Token::kLinebreak},
                                                       }}));

}  // namespace