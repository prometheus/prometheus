#pragma once

#include <string_view>

namespace PromPP::Prometheus::textparse {

enum class Token {
  kInvalid = -1,
  kEOF = 0,
  kLinebreak,
  kWhitespace,
  kHelp,
  kType,
  kUnit,
  kEOFWord,
  kText,
  kComment,
  kBlank,
  kMetricName,
  kQuotedString,
  kBraceOpen,
  kBraceClose,
  kLabelName,
  kLabelValue,
  kComma,
  kEqual,
  kTimestamp,
  kValue,
  kExemplar,
};

template <class Tokenizer>
concept TokenizerInterface = requires(Tokenizer& tokenizer, const Tokenizer& const_tokenizer) {
  { Tokenizer(std::string_view()) };

  { tokenizer.tokenize(std::string_view()) } -> std::same_as<void>;
  { tokenizer.next() } -> std::same_as<Token>;
  { tokenizer.next_non_whitespace() } -> std::same_as<Token>;
  { tokenizer.consume_comment(Token()) } -> std::same_as<Token>;

  { const_tokenizer.buffer() } -> std::same_as<std::string_view>;
  { const_tokenizer.token_str() } -> std::same_as<std::string_view>;
  { const_tokenizer.token() } -> std::same_as<Token>;
};

}  // namespace PromPP::Prometheus::textparse