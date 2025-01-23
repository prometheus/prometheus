#pragma once

#include <simdutf/simdutf.h>

#include "bare_bones/vector.h"
#include "parser.h"
#include "prometheus/hashdex.h"
#include "prometheus/metric.h"
#include "prometheus/textparse/escape.h"
#include "prometheus/value.h"

namespace PromPP::WAL::hashdex::scraper {

template <ParserInterface Parser>
class Scraper {
 public:
  [[nodiscard]] Error parse(std::span<char> buffer, Primitives::Timestamp default_timestamp) {
    metric_buffer_.initialize(buffer.size() / 2);
    metadata_buffer_.initialize(buffer.size() / 4);
    parser_.tokenizer().tokenize({buffer.data(), buffer.data() + buffer.size()});

    while (true) {
      switch (parser_.tokenizer().next()) {
        case Token::kEOF:
        case Token::kEOFWord: {
          return parser_.validate_parse_result();
        }

        case Token::kWhitespace:
        case Token::kLinebreak: {
          break;
        }

        case Token::kComment: {
          parser_.tokenizer().consume_comment();
          break;
        }

        case Token::kHelp:
        case Token::kUnit:
        case Token::kType: {
          if (const auto error = parse_metadata(); error != Error::kNoError) [[unlikely]] {
            return error;
          }
          break;
        }

        case Token::kMetricName:
        case Token::kBraceOpen: {
          if (const auto error = MetricParser{parser_, metric_buffer_, metric_buffer_.add_metric(default_timestamp)}.parse(); error != Error::kNoError)
              [[unlikely]] {
            metric_buffer_.remove_item();
            return error;
          }

          if (parser_.tokenizer().token() == Token::kExemplar) {
            parser_.tokenizer().consume_comment();
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
    [[nodiscard]] PROMPP_LAMBDA_INLINE auto begin() const noexcept { return scraper_.metric_buffer_.begin(scraper_.parser_.tokenizer().buffer()); }
    [[nodiscard]] PROMPP_LAMBDA_INLINE static auto end() noexcept { return MetricMarkupBuffer::end(); }

   private:
    const Scraper& scraper_;
  };

  class MetadataWrapper {
   public:
    explicit MetadataWrapper(const Scraper& scraper) : scraper_(scraper) {}

    [[nodiscard]] PROMPP_LAMBDA_INLINE uint32_t size() const noexcept { return scraper_.metadata_buffer_.items_count(); }
    [[nodiscard]] PROMPP_LAMBDA_INLINE auto begin() const noexcept { return scraper_.metadata_buffer_.begin(scraper_.parser_.tokenizer().buffer()); }
    [[nodiscard]] PROMPP_LAMBDA_INLINE static auto end() noexcept { return MetadataMarkupBuffer::end(); }

   private:
    const Scraper& scraper_;
  };

  [[nodiscard]] PROMPP_LAMBDA_INLINE uint32_t size() const noexcept { return metric_buffer_.items_count(); }
  [[nodiscard]] PROMPP_LAMBDA_INLINE auto begin() const noexcept { return metric_buffer_.begin(parser_.tokenizer().buffer()); }
  [[nodiscard]] PROMPP_LAMBDA_INLINE static auto end() noexcept { return MetricMarkupBuffer::end(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE MetricsWrapper metrics() const noexcept { return MetricsWrapper{*this}; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE MetadataWrapper metadata() const noexcept { return MetadataWrapper{*this}; }

 private:
  using Token = Prometheus::textparse::Token;

#pragma pack(push, 1)
  struct MarkedString {
    uint32_t offset{std::numeric_limits<uint32_t>::max()};
    uint32_t length{std::numeric_limits<uint32_t>::max()};

    [[nodiscard]] PROMPP_ALWAYS_INLINE static MarkedString create(const std::string_view& value, const std::string_view& buffer) noexcept {
      return {
          .offset = static_cast<uint32_t>(value.data() - buffer.data()),
          .length = static_cast<uint32_t>(value.size()),
      };
    }

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
      ++this->items_count_;

      const auto offset = this->buffer_.size();
      this->buffer_.resize(offset + sizeof(MarkedMetric));
      return new (reinterpret_cast<MarkedMetric*>(this->buffer_.data() + offset)) MarkedMetric(default_timestamp);
    }

    PROMPP_ALWAYS_INLINE void add_label(const MarkedLabel& label, MarkedMetric*& metric) noexcept {
      if (label.value.is_empty()) [[unlikely]] {
        return;
      }

      const auto offset = reinterpret_cast<const char*>(metric) - this->buffer_.data();
      this->buffer_.push_back(reinterpret_cast<const char*>(&label), reinterpret_cast<const char*>(&label) + sizeof(label));
      metric = reinterpret_cast<MarkedMetric*>(this->buffer_.data() + offset);
      ++metric->label_set.count;
    }
  };

  class MetadataMarkupBuffer : public MarkupBuffer<Metadata> {
   public:
    PROMPP_ALWAYS_INLINE void add(MarkedString metric_name, MarkedString text, Prometheus::MetadataType type) noexcept {
      ++this->items_count_;

      const auto offset = this->buffer_.size();
      this->buffer_.resize(offset + sizeof(MarkedMetadata));
      new (reinterpret_cast<MarkedMetadata*>(this->buffer_.data() + offset)) MarkedMetadata{
          .metric_name = metric_name,
          .text = text,
          .type = type,
      };
    }
  };

  class MetricParser {
   public:
    MetricParser(Parser& parser, MetricMarkupBuffer& markup_buffer, MarkedMetric* metric) : parser_(parser), markup_buffer_(markup_buffer), metric_(metric) {}

    [[nodiscard]] Error parse() noexcept {
      bool have_metric_name = false;
      auto& tokenizer = parser_.tokenizer();
      if (tokenizer.token() == Token::kMetricName) [[likely]] {
        markup_buffer_.add_label(MarkedLabel{.value = MarkedString::create(tokenizer.token_str(), tokenizer.buffer())}, metric_);
        have_metric_name = true;
        tokenizer.next_non_whitespace();
      } else if (tokenizer.token() == Token::kWhitespace) [[likely]] {
        tokenizer.next();
      }

      if (tokenizer.token() == Token::kBraceOpen) [[likely]] {
        if (const auto error = tokenize_label_set(have_metric_name); error != Error::kNoError) {
          return error;
        }

        tokenizer.next_non_whitespace();
      } else if (tokenizer.token() != Token::kValue) [[unlikely]] {
        return Error::kUnexpectedToken;
      }

      if (!have_metric_name) [[unlikely]] {
        return Error::kNoMetricName;
      }

      metric_->calculate_hash(tokenizer.buffer());
      return parse_metric_suffix();
    }

   private:
    Parser& parser_;
    MetricMarkupBuffer& markup_buffer_;
    MarkedMetric* metric_;

    [[nodiscard]] Error tokenize_label_set(bool& have_metric_name) noexcept {
      auto& tokenizer = parser_.tokenizer();
      tokenizer.next_non_whitespace();

      while (tokenizer.token() != Token::kBraceClose) {
        MarkedLabel label;
        if (const auto error = get_label_name(label.name); error != Error::kNoError) [[unlikely]] {
          return error;
        }

        if (tokenizer.next_non_whitespace() == Token::kEqual) [[likely]] {
          if (tokenizer.next_non_whitespace() != Token::kLabelValue) [[unlikely]] {
            return Error::kUnexpectedToken;
          }

          if (const auto error = get_quoted_value(label.value); error != Error::kNoError) [[unlikely]] {
            return error;
          }

          markup_buffer_.add_label(label, metric_);
          tokenizer.next();
        } else {
          if (!have_metric_name) [[unlikely]] {
            markup_buffer_.add_label(MarkedLabel{.value = label.name}, metric_);
            have_metric_name = true;
          } else {
            return Error::kUnexpectedToken;
          }
        }

        if (tokenizer.token() != Token::kComma && tokenizer.token() != Token::kWhitespace) {
          break;
        }

        tokenizer.next_non_whitespace();
      }

      return tokenizer.token() == Token::kBraceClose ? Error::kNoError : Error::kUnexpectedToken;
    }

    [[nodiscard]] Error get_label_name(MarkedString& label_name) const noexcept {
      auto& tokenizer = parser_.tokenizer();

      if (tokenizer.token() == Token::kLabelName) [[likely]] {
        label_name = MarkedString::create(tokenizer.token_str(), tokenizer.buffer());
        return Error::kNoError;
      }
      if (tokenizer.token() == Token::kQuotedString) {
        return get_quoted_value(label_name);
      }

      return Error::kUnexpectedToken;
    }

    [[nodiscard]] Error get_quoted_value(MarkedString& string) const noexcept {
      auto& tokenizer = parser_.tokenizer();

      auto value = tokenizer.token_str();
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

      string = MarkedString::create(value, tokenizer.buffer());
      return Error::kNoError;
    }

    [[nodiscard]] Error parse_metric_suffix() noexcept {
      if (!parser_.is_value_token()) [[unlikely]] {
        return Error::kUnexpectedToken;
      }

      if (const auto error = parse_sample(); error != Error::kNoError) {
        return error;
      }

      return parser_.validate_parse_sample_result();
    }

    [[nodiscard]] Error parse_sample() noexcept {
      auto& tokenizer = parser_.tokenizer();

      if (!parse_numeric_value(tokenizer.token_str(), metric_->sample.value())) [[unlikely]] {
        return Error::kInvalidValue;
      }
      if (std::isnan(metric_->sample.value())) [[unlikely]] {
        metric_->sample.value() = Prometheus::kNormalNan;
      }

      tokenizer.next_non_whitespace();

      return parser_.parse_timestamp(metric_->sample.timestamp());
    }
  };

  [[nodiscard]] Error parse_metadata() {
    static constexpr auto get_metadata_type = [](Token token) PROMPP_LAMBDA_INLINE {
      if (token == Token::kHelp) {
        return Prometheus::MetadataType::kHelp;
      }
      if (token == Token::kType) {
        return Prometheus::MetadataType::kType;
      }

      return Prometheus::MetadataType::kUnit;
    };

    auto& tokenizer = parser_.tokenizer();
    const auto type = tokenizer.token();

    if (tokenizer.next_non_whitespace() != Token::kMetricName) [[unlikely]] {
      return Error::kUnexpectedToken;
    }

    auto metric_name = tokenizer.token_str();
    Prometheus::textparse::unquote(metric_name);

    if (tokenizer.next_non_whitespace() != Token::kText) [[unlikely]] {
      return Error::kUnexpectedToken;
    }

    const auto text = tokenizer.token_str();
    if (const auto token = tokenizer.next_non_whitespace(); token != Token::kLinebreak) [[unlikely]] {
      return Error::kUnexpectedToken;
    }

    if (type == Token::kHelp && !simdutf::validate_utf8(text.data(), text.size())) [[unlikely]] {
      return Error::kInvalidUtf8;
    }

    const auto buffer = tokenizer.buffer();
    metadata_buffer_.add(MarkedString::create(metric_name, buffer), MarkedString::create(text, buffer), get_metadata_type(type));
    return Error::kNoError;
  }

  Parser parser_;
  MetricMarkupBuffer metric_buffer_;
  MetadataMarkupBuffer metadata_buffer_;
};

using PrometheusScraper = Scraper<PrometheusParser>;
using OpenMetricsScraper = Scraper<OpenMetricsParser>;

static_assert(Prometheus::hashdex::HashdexInterface<PrometheusScraper>);
static_assert(Prometheus::hashdex::HashdexInterface<OpenMetricsScraper>);

}  // namespace PromPP::WAL::hashdex::scraper
