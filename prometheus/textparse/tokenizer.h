#pragma once

#include <string_view>

namespace PromPP::Prometheus::textparse {

class Tokenizer {
 public:
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
  };

  Tokenizer();
  explicit Tokenizer(std::string_view str);

  void tokenize(std::string_view str) noexcept;

  Token next() noexcept {
    token_ = next_impl();
    return token_;
  }
  Token next_non_whitespace() noexcept {
    if (const auto token = next(); token == Token::kWhitespace) {
      return next();
    } else {
      return token;
    }
  }

  Token consume_comment() noexcept;

  [[nodiscard]] std::string_view buffer() const noexcept { return {start_ptr_, limit_ptr_}; }

  [[nodiscard]] std::string_view token_str() const noexcept { return {token_ptr_, static_cast<size_t>(cursor_ptr_ - token_ptr_)}; }
  [[nodiscard]] Token token() const noexcept { return token_; }

 private:
  const char* start_ptr_{};
  const char* cursor_ptr_{};
  const char* limit_ptr_{};
  const char* marker_ptr_{};
  const char* token_ptr_{};
  int condition_;
  Token token_{Token::kInvalid};

  Token next_impl() noexcept;
};

}  // namespace PromPP::Prometheus::textparse
