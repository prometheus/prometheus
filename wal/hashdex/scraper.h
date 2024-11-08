#pragma once

#include <fastfloat/fast_float.h>
#include <simdutf/simdutf.h>

#include "bare_bones/vector.h"
#include "primitives/primitives.h"
#include "prometheus/metadata.h"
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
    metric_buffer_.initialize(buffer.size() / 2);
    metadata_buffer_.initialize(buffer.size() / 4);
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

        case kComment: {
          tokenizer_.consume_comment();
          break;
        }

        case kHelp:
        case kType: {
          if (const auto error = parse_metadata(); error != Error::kNoError) [[unlikely]] {
            return error;
          }
          break;
        }

        case kMetricName:
        case kBraceOpen: {
          if (const auto error = MetricParser{tokenizer_, metric_buffer_, metric_buffer_.add_metric(default_timestamp)}.parse(); error != Error::kNoError)
              [[unlikely]] {
            metric_buffer_.remove_item();
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

  class MetricsWrapper {
   public:
    explicit MetricsWrapper(const Scraper& scraper) : scraper_(scraper) {}

    [[nodiscard]] PROMPP_LAMBDA_INLINE uint32_t size() const noexcept { return scraper_.metric_buffer_.items_count(); }
    [[nodiscard]] PROMPP_LAMBDA_INLINE auto begin() const noexcept { return scraper_.metric_buffer_.begin(scraper_.tokenizer_.buffer()); }
    [[nodiscard]] PROMPP_LAMBDA_INLINE static auto end() noexcept { return MetricMarkupBuffer::end(); }

   private:
    const Scraper& scraper_;
  };

  class MetadataWrapper {
   public:
    explicit MetadataWrapper(const Scraper& scraper) : scraper_(scraper) {}

    [[nodiscard]] PROMPP_LAMBDA_INLINE uint32_t size() const noexcept { return scraper_.metadata_buffer_.items_count(); }
    [[nodiscard]] PROMPP_LAMBDA_INLINE auto begin() const noexcept { return scraper_.metadata_buffer_.begin(scraper_.tokenizer_.buffer()); }
    [[nodiscard]] PROMPP_LAMBDA_INLINE static auto end() noexcept { return MetadataMarkupBuffer::end(); }

   private:
    const Scraper& scraper_;
  };

  [[nodiscard]] PROMPP_LAMBDA_INLINE uint32_t size() const noexcept { return metric_buffer_.items_count(); }
  [[nodiscard]] PROMPP_LAMBDA_INLINE auto begin() const noexcept { return metric_buffer_.begin(tokenizer_.buffer()); }
  [[nodiscard]] PROMPP_LAMBDA_INLINE static auto end() noexcept { return MetricMarkupBuffer::end(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE MetricsWrapper metrics() const noexcept { return MetricsWrapper{*this}; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE MetadataWrapper metadata() const noexcept { return MetadataWrapper{*this}; }

 private:
  using enum Tokenizer::Token;

#pragma pack(push, 1)
  struct MarkedString {
    uint32_t offset{std::numeric_limits<uint32_t>::max()};
    uint32_t length{std::numeric_limits<uint32_t>::max()};

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_valid() const noexcept {
      return offset != std::numeric_limits<uint32_t>::max() && length != std::numeric_limits<uint32_t>::max();
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const noexcept { return length == 0; }

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

  struct MarkedMetric {
    uint64_t hash{};
    Primitives::Sample sample{};
    MarkedLabelSet label_set;

    explicit MarkedMetric(Primitives::Timestamp timestamp) : sample(timestamp, 0.0) {}

    PROMPP_ALWAYS_INLINE void calculate_hash(const std::string_view& buffer) noexcept {
      label_set.sort(buffer);
      hash = label_set.hash(buffer);
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t occupied_size() const noexcept { return sizeof(*this) + sizeof(MarkedLabel) * label_set.count; }
  };

  struct MarkedMetadata {
    MarkedString metric_name{};
    MarkedString text{};
    Prometheus::MetadataType type{};

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t occupied_size() const noexcept { return sizeof(*this); }
  };
#pragma pack(pop)

 public:
  class Metric {
   public:
    using MarkedItem = MarkedMetric;

    explicit Metric(std::string_view buffer, const MarkedMetric* item) : buffer_(buffer), item_(item) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE const MarkedMetric* item() const noexcept { return item_; }
    PROMPP_ALWAYS_INLINE void set_item(const MarkedMetric* item) noexcept { item_ = item; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t hash() const noexcept { return item_->hash; }

    template <class Timeseries>
    void read(Timeseries& timeseries) const {
      timeseries.label_set().reserve(item_->label_set.count);
      for (uint32_t i = 0; i < item_->label_set.count; ++i) {
        const auto& [name, value] = item_->label_set.labels[i];
        timeseries.label_set().append(name.string_view(buffer_), value.string_view(buffer_));
      }

      timeseries.samples().emplace_back(item_->sample);
    }

   private:
    std::string_view buffer_;
    const MarkedMetric* item_{};
  };

  class Metadata {
   public:
    using MarkedItem = MarkedMetadata;

    explicit Metadata(std::string_view buffer, const MarkedMetadata* item) : buffer_(buffer), item_(item) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE const MarkedMetadata* item() const noexcept { return item_; }
    PROMPP_ALWAYS_INLINE void set_item(const MarkedMetadata* item) noexcept { item_ = item; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE Prometheus::MetadataType type() const noexcept { return item_->type; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view metric_name() const noexcept { return item_->metric_name.string_view(buffer_); }
    [[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view text() const noexcept { return item_->text.string_view(buffer_); }

   private:
    std::string_view buffer_;
    const MarkedMetadata* item_{};
  };

 private:
  template <class Item>
  class MarkupBuffer {
   public:
    class IteratorSentinel {};

    class Iterator {
     public:
      using iterator_category = std::forward_iterator_tag;
      using value_type = Item;
      using difference_type = ptrdiff_t;
      using pointer = value_type*;
      using reference = value_type&;
      using MarkedItem = typename Item::MarkedItem;

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

    PROMPP_ALWAYS_INLINE void remove_item() noexcept { --items_count_; }

    PROMPP_ALWAYS_INLINE void initialize(size_t reserve) noexcept {
      buffer_.clear();
      buffer_.reserve(reserve);
      items_count_ = 0;
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE Iterator begin(std::string_view buffer) const noexcept { return {buffer, this}; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

   protected:
    BareBones::Vector<char> buffer_;
    uint32_t items_count_{};
  };

  class MetricMarkupBuffer : public MarkupBuffer<Metric> {
   public:
    PROMPP_ALWAYS_INLINE MarkedMetric* add_metric(Primitives::Timestamp default_timestamp) noexcept {
      ++items_count_;

      const auto offset = buffer_.size();
      buffer_.resize(offset + sizeof(MarkedMetric));
      return new (reinterpret_cast<MarkedMetric*>(buffer_.data() + offset)) MarkedMetric(default_timestamp);
    }

    PROMPP_ALWAYS_INLINE void add_label(const MarkedLabel& label, MarkedMetric*& metric) noexcept {
      if (label.value.is_empty()) [[unlikely]] {
        return;
      }

      const auto offset = reinterpret_cast<const char*>(metric) - buffer_.data();
      buffer_.push_back(reinterpret_cast<const char*>(&label), reinterpret_cast<const char*>(&label) + sizeof(label));
      metric = reinterpret_cast<MarkedMetric*>(buffer_.data() + offset);
      ++metric->label_set.count;
    }
  };

  class MetadataMarkupBuffer : public MarkupBuffer<Metadata> {
   public:
    PROMPP_ALWAYS_INLINE void add(MarkedString metric_name, MarkedString text, Prometheus::MetadataType type) noexcept {
      ++items_count_;

      const auto offset = buffer_.size();
      buffer_.resize(offset + sizeof(MarkedMetadata));
      new (reinterpret_cast<MarkedMetadata*>(buffer_.data() + offset)) MarkedMetadata{
          .metric_name = metric_name,
          .text = text,
          .type = type,
      };
    }
  };

  class MetricParser {
   public:
    MetricParser(Tokenizer& tokenizer, MetricMarkupBuffer& markup_buffer, MarkedMetric* metric)
        : tokenizer_(tokenizer), markup_buffer_(markup_buffer), metric_(metric) {}

    [[nodiscard]] Error parse() noexcept {
      bool have_metric_name = false;
      if (tokenizer_.token() == kMetricName) [[likely]] {
        markup_buffer_.add_label(MarkedLabel{.value = create_marked_string(tokenizer_.token_str(), tokenizer_.buffer())}, metric_);
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

      metric_->calculate_hash(tokenizer_.buffer());
      return parse_metric_suffix();
    }

   private:
    Tokenizer& tokenizer_;
    MetricMarkupBuffer& markup_buffer_;
    MarkedMetric* metric_;

    [[nodiscard]] Error tokenize_metric_name_and_label_set() noexcept {
      if (tokenizer_.next_non_whitespace() != kQuotedString) {
        return Error::kNoMetricName;
      }

      MarkedLabel metric_name;
      if (const auto error = get_quoted_value(metric_name.value); error != Error::kNoError) {
        return error;
      }
      markup_buffer_.add_label(metric_name, metric_);

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

        markup_buffer_.add_label(label, metric_);
      } while (tokenizer_.next(), tokenizer_.token() == kComma || tokenizer_.token() == kWhitespace);

      return tokenizer_.token() == kBraceClose ? Error::kNoError : Error::kUnexpectedToken;
    }

    [[nodiscard]] Error get_label_name(MarkedString& label_name) const noexcept {
      if (tokenizer_.token() == kLabelName) [[likely]] {
        label_name = create_marked_string(tokenizer_.token_str(), tokenizer_.buffer());
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
      Prometheus::textparse::unescape_label_value(value, [&copy_to](const std::string_view& piece_of_string) {
        if (copy_to != piece_of_string.data()) [[unlikely]] {
          memmove(copy_to, piece_of_string.data(), piece_of_string.size());
        }

        copy_to += piece_of_string.size();
      });
      value.remove_suffix(value.size() - (copy_to - value.data()));

      if (!simdutf::validate_utf8(value.data(), value.size())) [[unlikely]] {
        return Error::kInvalidUtf8;
      }

      string = create_marked_string(value, tokenizer_.buffer());
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
      if (!parse_token_value(metric_->sample.value())) [[unlikely]] {
        return Error::kInvalidValue;
      }
      if (std::isnan(metric_->sample.value())) [[unlikely]] {
        metric_->sample.value() = Prometheus::kNormalNan;
      }

      tokenizer_.next_non_whitespace();

      if (tokenizer_.token() == kTimestamp || (tokenizer_.token() == kEOF && !tokenizer_.token_str().empty())) {
        if (!parse_token_value(metric_->sample.timestamp())) {
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

  [[nodiscard]] Error parse_metadata() {
    const auto type = tokenizer_.token();

    if (tokenizer_.next_non_whitespace() != kMetricName) [[unlikely]] {
      return Error::kUnexpectedToken;
    }

    auto metric_name = tokenizer_.token_str();
    Prometheus::textparse::unquote(metric_name);

    if (tokenizer_.next_non_whitespace() != kText) [[unlikely]] {
      return Error::kUnexpectedToken;
    }

    const auto text = tokenizer_.token_str();
    if (const auto token = tokenizer_.next_non_whitespace(); token != kLinebreak) [[unlikely]] {
      return Error::kUnexpectedToken;
    }

    if (type == kHelp && !simdutf::validate_utf8(text.data(), text.size())) [[unlikely]] {
      return Error::kInvalidUtf8;
    }

    const auto buffer = tokenizer_.buffer();
    metadata_buffer_.add(create_marked_string(metric_name, buffer), create_marked_string(text, buffer),
                         type == kHelp ? Prometheus::MetadataType::kHelp : Prometheus::MetadataType::kType);
    return Error::kNoError;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static MarkedString create_marked_string(const std::string_view& value, const std::string_view& buffer) noexcept {
    return {
        .offset = static_cast<uint32_t>(value.data() - buffer.data()),
        .length = static_cast<uint32_t>(value.size()),
    };
  }

  Prometheus::textparse::Tokenizer tokenizer_;
  MetricMarkupBuffer metric_buffer_;
  MetadataMarkupBuffer metadata_buffer_;
};

}  // namespace PromPP::WAL::hashdex
