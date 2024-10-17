#pragma once

#include <fastfloat/fast_float.h>
#include <utf8/utf8.h>

#include "bare_bones/vector.h"
#include "primitives/primitives.h"
#include "prometheus/textparse/escape.h"
#include "prometheus/textparse/tokenizer.h"
#include "prometheus/value.h"

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

  [[nodiscard]] Error parse(std::span<char> buffer, Primitives::Timestamp default_timestamp) {
    markup_buffer_.initialize(buffer.size() / 2);
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
          if (const auto error = ItemParser{tokenizer_, markup_buffer_, markup_buffer_.add_item(default_timestamp)}.parse(); error != Error::kNoError) {
            markup_buffer_.remove_item();
            return error;
          }
          break;
        }

        default: {
          return Error::kUnexpectedToken;
        }
      }
    }
  }

  [[nodiscard]] PROMPP_LAMBDA_INLINE uint32_t size() const noexcept { return markup_buffer_.items_count(); }
  [[nodiscard]] PROMPP_LAMBDA_INLINE auto begin() const noexcept { return markup_buffer_.begin(tokenizer_.buffer()); }
  [[nodiscard]] PROMPP_LAMBDA_INLINE static auto end() noexcept { return MarkupBuffer::end(); }

 private:
  using enum Tokenizer::Token;

#pragma pack(push, 1)
  struct MarkedString {
    uint32_t offset{std::numeric_limits<uint32_t>::max()};
    uint32_t length{std::numeric_limits<uint32_t>::max()};

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_valid() const noexcept {
      return offset != std::numeric_limits<uint32_t>::max() && length != std::numeric_limits<uint32_t>::max();
    }

    [[nodiscard]] std::string_view string_view(const std::string_view& buffer) const noexcept {
      if (!is_valid()) [[unlikely]] {
        return Prometheus::kMetricLabelName;
      }

      return buffer.substr(offset, length);
    }
  };

  struct MarkedLabel {
    MarkedString name{};
    MarkedString value;
  };

  struct MarkedLabelSet {
    uint32_t count{};
    MarkedLabel labels[];

    PROMPP_ALWAYS_INLINE void sort(const std::string_view& buffer) noexcept {
      std::sort(labels, labels + count,
                [&buffer](const MarkedLabel& a, const MarkedLabel& b) PROMPP_LAMBDA_INLINE { return a.name.string_view(buffer) < b.name.string_view(buffer); });
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t hash(const std::string_view& buffer) const noexcept {
      BareBones::XXHash hash;
      for (uint32_t i = 0; i < count; ++i) {
        const auto& [name, value] = labels[i];
        hash.extend(name.string_view(buffer), value.string_view(buffer));
      }
      return hash.hash();
    }
  };

  struct MarkedItem {
    uint64_t hash{};
    Primitives::Sample sample{};
    MarkedLabelSet label_set;

    explicit MarkedItem(Primitives::Timestamp timestamp) : sample(timestamp, 0.0) {}

    PROMPP_ALWAYS_INLINE void calculate_hash(const std::string_view& buffer) noexcept {
      label_set.sort(buffer);
      hash = label_set.hash(buffer);
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t occupied_size() const noexcept { return sizeof(*this) + sizeof(MarkedLabel) * label_set.count; }
  };
#pragma pack(pop)

  class MarkupBuffer {
   public:
    class Item {
     public:
      explicit Item(std::string_view buffer, const MarkedItem* item) : buffer_(buffer), item_(item) {}

      [[nodiscard]] PROMPP_ALWAYS_INLINE const MarkedItem* item() const noexcept { return item_; }
      PROMPP_ALWAYS_INLINE void set_item(const MarkedItem* item) noexcept { item_ = item; }

      [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t hash() const noexcept { return item_->hash; }

      template <class Timeseries>
      void read(Timeseries& timeseries) const {
        timeseries.label_set().reserve(item_->label_set.count);
        for (uint32_t i = 0; i < item_->label_set.count; ++i) {
          const auto& [name, value] = item_->label_set.labels[i];
          timeseries.label_set().add({name.string_view(buffer_), value.string_view(buffer_)});
        }

        timeseries.samples().emplace_back(item_->sample);
      }

     private:
      std::string_view buffer_;
      const MarkedItem* item_{};
    };

    class IteratorSentinel {};

    class Iterator {
     public:
      using iterator_category = std::forward_iterator_tag;
      using value_type = Item;
      using difference_type = ptrdiff_t;
      using pointer = value_type*;
      using reference = value_type&;

      Iterator(std::string_view buffer, const MarkupBuffer* markup_buffer)
          : item_(buffer, reinterpret_cast<const MarkedItem*>(markup_buffer->buffer().data())), items_count_(markup_buffer->items_count()) {}

      [[nodiscard]] PROMPP_ALWAYS_INLINE const value_type& operator*() const noexcept { return item_; }
      [[nodiscard]] PROMPP_ALWAYS_INLINE const value_type* operator->() const noexcept { return &item_; }

      PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
        item_.set_item(reinterpret_cast<const MarkedItem*>(reinterpret_cast<const char*>(item_.item()) + item_.item()->occupied_size()));
        --items_count_;
        return *this;
      }

      PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
        const auto it = *this;
        ++*this;
        return it;
      }

      PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return items_count_ == 0; }

     private:
      Item item_;
      uint32_t items_count_;
    };

    [[nodiscard]] PROMPP_ALWAYS_INLINE const BareBones::Vector<char>& buffer() const noexcept { return buffer_; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t items_count() const noexcept { return items_count_; }

    PROMPP_ALWAYS_INLINE MarkedItem* add_item(Primitives::Timestamp default_timestamp) noexcept {
      ++items_count_;

      const auto offset = buffer_.size();
      buffer_.resize(offset + sizeof(MarkedItem));
      return new (reinterpret_cast<MarkedItem*>(buffer_.data() + offset)) MarkedItem(default_timestamp);
    }

    PROMPP_ALWAYS_INLINE void remove_item() noexcept { --items_count_; }

    PROMPP_ALWAYS_INLINE void add_label(const MarkedLabel& label, MarkedItem*& item) noexcept {
      const auto offset = reinterpret_cast<const char*>(item) - buffer_.data();
      buffer_.push_back(reinterpret_cast<const char*>(&label), reinterpret_cast<const char*>(&label) + sizeof(label));
      item = reinterpret_cast<MarkedItem*>(buffer_.data() + offset);
      ++item->label_set.count;
    }

    PROMPP_ALWAYS_INLINE void initialize(size_t reserve) noexcept {
      buffer_.clear();
      buffer_.reserve(reserve);
      items_count_ = 0;
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE Iterator begin(std::string_view buffer) const noexcept { return {buffer, this}; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

   private:
    BareBones::Vector<char> buffer_;
    uint32_t items_count_{};
  };

  class ItemParser {
   public:
    ItemParser(Tokenizer& tokenizer, MarkupBuffer& markup_buffer, MarkedItem* item) : tokenizer_(tokenizer), markup_buffer_(markup_buffer), item_(item) {}

    [[nodiscard]] Error parse() noexcept {
      bool have_metric_name = false;
      if (tokenizer_.token() == kMetricName) [[likely]] {
        markup_buffer_.add_label(MarkedLabel{.value = create_marked_string(tokenizer_.token_str())}, item_);
        have_metric_name = true;
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

      item_->calculate_hash(tokenizer_.buffer());
      return parse_metric_suffix();
    }

   private:
    Tokenizer& tokenizer_;
    MarkupBuffer& markup_buffer_;
    MarkedItem* item_;

    [[nodiscard]] PROMPP_ALWAYS_INLINE MarkedString create_marked_string(const std::string_view& value) const noexcept {
      return {
          .offset = static_cast<uint32_t>(value.data() - tokenizer_.buffer().data()),
          .length = static_cast<uint32_t>(value.size()),
      };
    }

    [[nodiscard]] Error tokenize_metric_name_and_label_set() noexcept {
      if (tokenizer_.next_non_whitespace() != kQuotedString) {
        return Error::kNoMetricName;
      }

      MarkedLabel metric_name;
      if (const auto error = get_quoted_value(metric_name.value); error != Error::kNoError) {
        return error;
      }
      markup_buffer_.add_label(metric_name, item_);

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

        MarkedLabel label;
        if (const auto error = get_label_name(label.name); error != Error::kNoError) [[unlikely]] {
          return error;
        }

        if (tokenizer_.next_non_whitespace() != kEqual) [[unlikely]] {
          return Error::kUnexpectedToken;
        }

        if (tokenizer_.next_non_whitespace() != kLabelValue) [[unlikely]] {
          return Error::kUnexpectedToken;
        }

        if (const auto error = get_quoted_value(label.value); error != Error::kNoError) [[unlikely]] {
          return error;
        }

        markup_buffer_.add_label(label, item_);
      } while (tokenizer_.next(), tokenizer_.token() == kComma || tokenizer_.token() == kWhitespace);

      return tokenizer_.token() == kBraceClose ? Error::kNoError : Error::kUnexpectedToken;
    }

    [[nodiscard]] Error get_label_name(MarkedString& label_name) const noexcept {
      if (tokenizer_.token() == kLabelName) [[likely]] {
        label_name = create_marked_string(tokenizer_.token_str());
        return Error::kNoError;
      }
      if (tokenizer_.token() == kQuotedString) {
        return get_quoted_value(label_name);
      }

      return Error::kUnexpectedToken;
    }

    [[nodiscard]] Error get_quoted_value(MarkedString& string) const noexcept {
      auto value = tokenizer_.token_str();
      Prometheus::textparse::unquote(value);

      auto copy_to = const_cast<char*>(value.data());
      auto error = Error::kNoError;
      Prometheus::textparse::unescape_label_value(value, [&copy_to, &error](const std::string_view& piece_of_string) {
        if (!UTF8::check_string_view_is_valid(piece_of_string)) [[unlikely]] {
          error = Error::kInvalidUtf8;
          return false;
        }

        if (copy_to != piece_of_string.data()) [[unlikely]] {
          memmove(copy_to, piece_of_string.data(), piece_of_string.size());
        }

        copy_to += piece_of_string.size();
        return true;
      });
      if (error != Error::kNoError) [[unlikely]] {
        return error;
      }

      value.remove_suffix(value.size() - (copy_to - value.data()));

      string = create_marked_string(value);
      return Error::kNoError;
    }

    [[nodiscard]] Error parse_metric_suffix() noexcept {
      if (tokenizer_.token() != kValue && tokenizer_.token() != kEOF) [[unlikely]] {
        return Error::kUnexpectedToken;
      }

      if (const auto error = parse_sample(); error != Error::kNoError) {
        return error;
      }

      return (tokenizer_.token() == kLinebreak || tokenizer_.token() == kEOF) ? Error::kNoError : Error::kUnexpectedToken;
    }

    [[nodiscard]] Error parse_sample() noexcept {
      if (!parse_token_value(item_->sample.value())) [[unlikely]] {
        return Error::kInvalidValue;
      }
      if (std::isnan(item_->sample.value())) [[unlikely]] {
        item_->sample.value() = Prometheus::kNormalNan;
      }

      tokenizer_.next_non_whitespace();

      if (tokenizer_.token() == kTimestamp || (tokenizer_.token() == kEOF && !tokenizer_.token_str().empty())) {
        if (!parse_token_value(item_->sample.timestamp())) {
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

      if (const auto [ptr, ec] = fast_float::from_chars(str.begin(), str.end(), value); ec != std::errc{} || ptr != str.end()) [[unlikely]] {
        return false;
      }

      return true;
    }
  };

  Prometheus::textparse::Tokenizer tokenizer_;
  MarkupBuffer markup_buffer_;
};

}  // namespace PromPP::WAL::hashdex