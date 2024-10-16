#pragma once

#include <charconv>

#include "bare_bones/vector.h"
#include "primitives/primitives.h"
#include "prometheus/textparse/escape.h"
#include "prometheus/textparse/tokenizer.h"
#include "prometheus/value.h"
#include "utf8/utf8.h"

namespace PromPP::WAL::hashdex {

class Scraper {
 public:
  using Tokenizer = Prometheus::textparse::Tokenizer;

  enum class Error : uint32_t {
    kNoError = 0,
    kUnexpectedToken,
    kNoMetricName,
    kInvalidUtf8,
    kInvalidValue,
    kInvalidTimestamp,
  };

  [[nodiscard]] Error parse(std::span<char> buffer, Primitives::Timestamp default_timestamp, std::string_view target_id_label) {
    items_.clear();
    target_id_label_ = target_id_label;
    tokenizer_.tokenize({buffer.data(), buffer.data() + buffer.size()});

    while (true) {
      switch (tokenizer_.next()) {
        case kEOF: {
          return Error::kNoError;
        }

        case kWhitespace:
        case kLinebreak: {
          break;
        }

        case kComment:
        case kHelp:
        case kType: {
          tokenizer_.consume_comment();
          break;
        }

        case kMetricName:
        case kBraceOpen: {
          if (const auto error = item_parser_.parse(default_timestamp); error != Error::kNoError) {
            return error;
          }
          items_.emplace_back(item_parser_.item());
          break;
        }

        default: {
          return Error::kUnexpectedToken;
        }
      }
    }
  }

  [[nodiscard]] PROMPP_LAMBDA_INLINE uint32_t size() const noexcept { return items_.size(); }
  [[nodiscard]] PROMPP_LAMBDA_INLINE auto begin() const noexcept { return items_.begin(); }
  [[nodiscard]] PROMPP_LAMBDA_INLINE auto end() const noexcept { return items_.end(); }
  [[nodiscard]] PROMPP_LAMBDA_INLINE std::string_view target_id_label() const noexcept { return target_id_label_; }

 private:
  using enum Tokenizer::Token;

  template <class LabelSet>
  class LabelSetParser {
   public:
    LabelSetParser(std::string_view buffer, LabelSet& label_set) : tokenizer_{buffer}, label_set_{label_set} {}

    void parse() noexcept {
      using enum Tokenizer::Token;

      while (true) {
        switch (tokenizer_.next()) {
          case kQuotedString: {
            if (!metric_name_added()) [[unlikely]] {
              add_metric_name_label<true>();
            } else {
              label_name_ = unescape_token();
            }
            break;
          }

          case kMetricName: {
            add_metric_name_label<false>();
            break;
          }

          case kLabelValue: {
            label_set_.add({label_name_, unescape_token()});
            break;
          }

          case kLabelName: {
            label_name_ = tokenizer_.token_str();
            break;
          }

          case kEOF: {
            return;
          }

          default: {
            break;
          }
        }
      }
    }

   private:
    std::string_view label_name_;
    Tokenizer tokenizer_;
    LabelSet& label_set_;

    [[nodiscard]] PROMPP_LAMBDA_INLINE bool metric_name_added() const noexcept { return !label_name_.empty(); }

    template <bool quoted>
    PROMPP_ALWAYS_INLINE void add_metric_name_label() {
      label_name_ = Prometheus::kMetricLabelName;
      if constexpr (quoted) {
        label_set_.add({label_name_, unescape_token()});
      } else {
        label_set_.add({label_name_, tokenizer_.token_str()});
      }
    }

    std::string_view unescape_token() noexcept {
      auto string = tokenizer_.token_str();
      Prometheus::textparse::unquote(string);

      auto copy_to = const_cast<char*>(string.data());
      Prometheus::textparse::unescape_label_value(string, [&copy_to](const std::string_view& piece_of_string) {
        memmove(copy_to, piece_of_string.data(), piece_of_string.size());
        copy_to += piece_of_string.size();
      });

      string.remove_suffix(string.size() - (copy_to - string.data()));
      return string;
    }
  };

  class ItemParser;

  class Item {
   public:
    explicit Item(const Scraper* scraper) : scraper_(scraper) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t hash() const noexcept { return hash_; }

    template <class Timeseries>
    void read(Timeseries& timeseries) const {
      LabelSetParser{buffer_, timeseries.label_set()}.parse();
      add_target_id_label<decltype(timeseries.label_set()), std::string_view>(timeseries.label_set());
      timeseries.samples().emplace_back(sample_);
    }

    template <class LabelSet, class StringType>
    void add_target_id_label(LabelSet& label_set) const {
      label_set.add({StringType{"__target_id__"}, StringType{scraper_->target_id_label()}});
    }

   private:
    std::string_view buffer_{};
    Primitives::Sample sample_{};
    uint64_t hash_{};
    const Scraper* scraper_{};

    friend class ItemParser;

    PROMPP_ALWAYS_INLINE void reset(Primitives::Timestamp timestamp, std::string_view buffer) noexcept {
      buffer_ = buffer;
      sample_ = Primitives::Sample(timestamp, 0.0);
      hash_ = 0;
    }

    PROMPP_ALWAYS_INLINE void set_buffer_length(size_t length) noexcept { buffer_ = {buffer_.data(), length}; }
  };

  class ItemParser {
   public:
    explicit ItemParser(Prometheus::textparse::Tokenizer& tokenizer, const Scraper* scraper) : tokenizer_(tokenizer), item_(scraper) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE const Item& item() const noexcept { return item_; }

    [[nodiscard]] Error parse(Primitives::Timestamp default_timestamp) noexcept {
      reset(default_timestamp);

      bool have_metric_name = false;
      if (tokenizer_.token() == kMetricName) [[likely]] {
        have_metric_name = true;
        label_set_.add({StringView{Prometheus::kMetricLabelName}, StringView{tokenizer_.token_str()}});
        tokenizer_.next();
      }

      if (tokenizer_.token() == kWhitespace) {
        tokenizer_.next();
      } else if (tokenizer_.token() != kBraceOpen) {
        return Error::kUnexpectedToken;
      }

      if (tokenizer_.token() == kBraceOpen) {
        if (const auto error = have_metric_name ? tokenize_label_set() : tokenize_metric_name_and_label_set(); error != Error::kNoError) {
          return error;
        }

        tokenizer_.next_non_whitespace();
      }

      item_.add_target_id_label<decltype(label_set_), StringView>(label_set_);
      item_.hash_ = hash_value(label_set_);
      return parse_metric_suffix();
    }

   private:
    using UnreplacedStringBuffer = BareBones::Vector<char>;

    class BufferView {
     public:
      BufferView(const UnreplacedStringBuffer& buffer, uint32_t offset, uint32_t length) : buffer_(&buffer), offset_(offset), length_(length) {}

      [[nodiscard]] PROMPP_ALWAYS_INLINE bool empty() const noexcept { return length_ == 0; }

      explicit operator std::string_view() const noexcept { return {buffer_->data() + offset_, buffer_->data() + offset_ + length_}; }

      auto operator<=>(const BufferView& other) const noexcept { return static_cast<std::string_view>(*this) <=> static_cast<std::string_view>(other); }
      bool operator==(const BufferView& other) const noexcept { return static_cast<std::string_view>(*this) == static_cast<std::string_view>(other); }

     private:
      const UnreplacedStringBuffer* buffer_;
      uint32_t offset_{};
      uint32_t length_{};
    };

    class StringView {
     public:
      StringView() : type_(Type::kStringView) {}
      explicit StringView(std::string_view str) : string_view_{str}, type_(Type::kStringView) {}
      explicit StringView(const UnreplacedStringBuffer& buffer, uint32_t offset, uint32_t length)
          : buffer_view_{buffer, offset, length}, type_(Type::kBufferView) {}

      void set(std::string_view str) noexcept {
        new (&string_view_) std::string_view{str};
        type_ = Type::kStringView;
      }

      void set(const UnreplacedStringBuffer& buffer, uint32_t offset, uint32_t length) noexcept {
        new (&buffer_view_) BufferView{buffer, offset, length};
        type_ = Type::kBufferView;
      }

      [[nodiscard]] PROMPP_ALWAYS_INLINE bool empty() const noexcept { return type_ == Type::kStringView ? string_view_.empty() : buffer_view_.empty(); }

      explicit operator std::string_view() const noexcept { return type_ == Type::kStringView ? string_view_ : static_cast<std::string_view>(buffer_view_); }

      auto operator<=>(const StringView& other) const noexcept { return static_cast<std::string_view>(*this) <=> static_cast<std::string_view>(other); }
      bool operator==(const StringView& other) const noexcept { return static_cast<std::string_view>(*this) == static_cast<std::string_view>(other); }

     private:
      enum class Type {
        kBufferView = 0,
        kStringView,
      };

      union {
        BufferView buffer_view_;
        std::string_view string_view_{};
      };
      Type type_;
    };

    using Label = std::pair<StringView, StringView>;
    using LabelSet = Primitives::BasicLabelSet<Label>;

    LabelSet label_set_{};
    UnreplacedStringBuffer unreplaced_string_buffer_{};
    Prometheus::textparse::Tokenizer& tokenizer_;
    Item item_;

    PROMPP_ALWAYS_INLINE void reset(Primitives::Timestamp default_timestamp) noexcept {
      item_.reset(default_timestamp, tokenizer_.token_str());
      label_set_.clear();
      unreplaced_string_buffer_.clear();
    }

    [[nodiscard]] Error tokenize_metric_name_and_label_set() noexcept {
      if (tokenizer_.next_non_whitespace() != kQuotedString) {
        return Error::kNoMetricName;
      }

      StringView label_name;
      if (const auto error = get_quoted_value(label_name); error != Error::kNoError) {
        return error;
      }
      label_set_.add({StringView{Prometheus::kMetricLabelName}, label_name});

      if (tokenizer_.next() == kBraceClose) {
        return Error::kNoError;
      }

      if (tokenizer_.token() != kComma && tokenizer_.token() != kWhitespace) {
        return Error::kUnexpectedToken;
      }

      return tokenize_label_set();
    }

    [[nodiscard]] Error tokenize_label_set() noexcept {
      do {
        if (tokenizer_.next_non_whitespace() == kBraceClose) {
          return Error::kNoError;
        }

        Label label;
        if (const auto error = get_label_name(label.first); error != Error::kNoError) [[unlikely]] {
          return error;
        }

        if (tokenizer_.next_non_whitespace() != kEqual) [[unlikely]] {
          return Error::kUnexpectedToken;
        }

        if (tokenizer_.next_non_whitespace() != kLabelValue) [[unlikely]] {
          return Error::kUnexpectedToken;
        }

        if (const auto error = get_quoted_value(label.second); error != Error::kNoError) [[unlikely]] {
          return error;
        }

        label_set_.add(label);
      } while (tokenizer_.next(), tokenizer_.token() == kComma || tokenizer_.token() == kWhitespace);

      return tokenizer_.token() == kBraceClose ? Error::kNoError : Error::kUnexpectedToken;
    }

    [[nodiscard]] Error get_label_name(StringView& label_name) noexcept {
      if (tokenizer_.token() == kLabelName) [[likely]] {
        label_name.set(tokenizer_.token_str());
        return Error::kNoError;
      }
      if (tokenizer_.token() == kQuotedString) {
        return get_quoted_value(label_name);
      }

      return Error::kUnexpectedToken;
    }

    [[nodiscard]] Error get_quoted_value(StringView& label_value) noexcept {
      auto value = tokenizer_.token_str();
      Prometheus::textparse::unquote(value);

      unreplaced_string_buffer_.reserve(unreplaced_string_buffer_.size() + value.length());
      const auto offset = unreplaced_string_buffer_.size();

      auto error = Error::kNoError;
      Prometheus::textparse::unescape_label_value(value, [this, &error](std::string_view piece_of_label_value) PROMPP_LAMBDA_INLINE {
        if (!UTF8::check_string_view_is_valid(piece_of_label_value)) [[likely]] {
          error = Error::kInvalidUtf8;
          return false;
        }

        unreplaced_string_buffer_.push_back(piece_of_label_value.begin(), piece_of_label_value.end());
        return true;
      });

      label_value.set(unreplaced_string_buffer_, static_cast<uint32_t>(offset), static_cast<uint32_t>(unreplaced_string_buffer_.size() - offset));
      return error;
    }

    [[nodiscard]] Error parse_metric_suffix() noexcept {
      if (tokenizer_.token() != kValue && tokenizer_.token() != kEOF) {
        return Error::kUnexpectedToken;
      }

      item_.set_buffer_length(tokenizer_.token_str().data() - item_.buffer_.data());

      if (const auto error = parse_sample(); error != Error::kNoError) {
        return error;
      }

      return (tokenizer_.token() == kLinebreak || tokenizer_.token() == kEOF) ? Error::kNoError : Error::kUnexpectedToken;
    }

    [[nodiscard]] Error parse_sample() noexcept {
      if (!parse_token_value(item_.sample_.value())) {
        return Error::kInvalidValue;
      }
      if (std::isnan(item_.sample_.value())) {
        item_.sample_.value() = Prometheus::kNormalNan;
      }

      tokenizer_.next_non_whitespace();

      if (tokenizer_.token() == kTimestamp || (tokenizer_.token() == kEOF && !tokenizer_.token_str().empty())) {
        if (!parse_token_value(item_.sample_.timestamp())) {
          return Error::kInvalidTimestamp;
        }

        tokenizer_.next_non_whitespace();
      }

      return Error::kNoError;
    }

    template <class Value>
    [[nodiscard]] bool parse_token_value(Value& value) noexcept {
      auto str = tokenizer_.token_str();
      if (str.front() == '+') {
        str.remove_prefix(1);
      }

      if (const auto [ptr, ec] = std::from_chars(str.begin(), str.end(), value); ec != std::errc{} || ptr != str.end()) {
        return false;
      }

      return true;
    }
  };

  BareBones::Vector<Item> items_;
  Prometheus::textparse::Tokenizer tokenizer_;
  ItemParser item_parser_{tokenizer_, this};
  std::string target_id_label_;
};

}  // namespace PromPP::WAL::hashdex