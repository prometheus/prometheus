#pragma once

#include <string>
#include <vector>

#include "third_party/protozero/pbf_reader.hpp"
#include "third_party/protozero/pbf_writer.hpp"

#include "selector.h"
#include "types.h"

namespace PromPP::Prometheus::Remote {

class Tag {
 public:
  enum Query { kStartTimestampMs = 1, kEndTimestampMs = 2, kLabelMatchers = 3, kHints = 4 };
  enum LabelMatcher { kType = 1, kName = 2, kValue = 3 };
  enum LabelValuesRequest { kQuery = 1, kLabelName = 2 };
};

class ProtobufReader {
 public:
  static bool read_label_matcher(protozero::pbf_reader reader, LabelMatcher& label_matcher) noexcept {
    label_matcher.set_default_protobuf_values();
    return noexcept_read([&]() PROMPP_LAMBDA_INLINE {
      while (reader.next()) {
        switch (reader.tag()) {
          case Tag::LabelMatcher::kType: {
            label_matcher.type = LabelMatcher::as_type(reader.get_enum());
            break;
          }

          case Tag::LabelMatcher::kName: {
            label_matcher.name = reader.get_string();
            break;
          }

          case Tag::LabelMatcher::kValue: {
            label_matcher.value = reader.get_string();
            break;
          }

          default: {
            reader.skip();
            break;
          }
        }
      }

      return label_matcher.is_valid();
    });
  }

  PROMPP_ALWAYS_INLINE static bool read_label_matcher(protozero::pbf_reader reader, Selector& selector) {
    return read_label_matcher(reader, selector.matchers.emplace_back().matcher);
  }

  static bool read_query(protozero::pbf_reader reader, Query& query) noexcept {
    return noexcept_read([&]() PROMPP_LAMBDA_INLINE {
      while (reader.next()) {
        switch (reader.tag()) {
          case Tag::Query::kStartTimestampMs: {
            query.start_timestamp_ms = reader.get_int64();
            break;
          }

          case Tag::Query::kEndTimestampMs: {
            query.end_timestamp_ms = reader.get_int64();
            break;
          }

          case Tag::Query::kLabelMatchers: {
            if (!read_label_matcher(reader.get_message(), query.selector)) {
              return false;
            }
            break;
          }

          default: {
            reader.skip();
            break;
          }
        }
      }

      return true;
    });
  }

  static bool read_label_values_query(protozero::pbf_reader reader, LabelValuesQuery& query) {
    return noexcept_read([&]() PROMPP_LAMBDA_INLINE {
      while (reader.next()) {
        switch (reader.tag()) {
          case Tag::LabelValuesRequest::kQuery: {
            if (!read_query(reader.get_message(), query.query)) {
              return false;
            }
            break;
          }

          case Tag::LabelValuesRequest::kLabelName: {
            query.label_name = reader.get_string();
            break;
          }

          default: {
            reader.skip();
            break;
          }
        }
      }

      return !query.label_name.empty();
    });
  }

 private:
  template <class ReadMethod>
  PROMPP_ALWAYS_INLINE static bool noexcept_read(ReadMethod&& read) noexcept {
    try {
      return read();
    } catch (...) {
      return false;
    }
  }
};

class ProtobufWriter {
 public:
  PROMPP_ALWAYS_INLINE static std::string write_query(const Query& query) {
    std::string request;
    protozero::pbf_writer writer(request);
    write_query(writer, query);
    return request;
  }

  template <class Buffer>
  PROMPP_ALWAYS_INLINE static void write_query(protozero::basic_pbf_writer<Buffer>& writer, const Query& query) {
    writer.add_int64(Tag::Query::kStartTimestampMs, query.start_timestamp_ms);
    writer.add_int64(Tag::Query::kEndTimestampMs, query.end_timestamp_ms);
    write_selector(writer, Tag::Query::kLabelMatchers, query.selector);
  }

  template <class Buffer>
  PROMPP_ALWAYS_INLINE static void write_selector(protozero::basic_pbf_writer<Buffer>& writer, protozero::pbf_tag_type tag, const Selector& selector) {
    for (auto& matcher : selector.matchers) {
      write_label_matcher(writer, tag, matcher.matcher);
    }
  }

  template <class Buffer>
  PROMPP_ALWAYS_INLINE static void write_label_matcher(protozero::basic_pbf_writer<Buffer>& writer,
                                                       protozero::pbf_tag_type tag,
                                                       const LabelMatcher& label_matcher) {
    protozero::basic_pbf_writer<Buffer> label_matcher_writer(writer, tag);

    label_matcher_writer.add_enum(Tag::LabelMatcher::kType, static_cast<uint32_t>(label_matcher.type));
    label_matcher_writer.add_string(Tag::LabelMatcher::kName, label_matcher.name);
    label_matcher_writer.add_string(Tag::LabelMatcher::kValue, label_matcher.value);
  }

  template <class StringList>
  PROMPP_ALWAYS_INLINE static std::string write_repeated_string(protozero::pbf_tag_type tag, const StringList& strings) {
    std::string request;
    protozero::pbf_writer writer(request);
    write_repeated_string(writer, tag, strings);
    return request;
  }

  template <class Buffer, class StringList>
  PROMPP_ALWAYS_INLINE static void write_repeated_string(protozero::basic_pbf_writer<Buffer>& writer, protozero::pbf_tag_type tag, const StringList& strings) {
    for (auto& string : strings) {
      writer.add_string(tag, string);
    }
  }

  PROMPP_ALWAYS_INLINE static std::string write_label_values_query(const LabelValuesQuery& query) {
    std::string request;
    protozero::pbf_writer writer(request);
    write_label_values_query(writer, query);
    return request;
  }

  template <class Buffer>
  PROMPP_ALWAYS_INLINE static void write_label_values_query(protozero::basic_pbf_writer<Buffer>& writer, const LabelValuesQuery& query) {
    protozero::basic_pbf_writer<Buffer> query_writer(writer, Tag::LabelValuesRequest::kQuery);
    write_query(query_writer, query.query);
    query_writer.commit();

    writer.add_string(Tag::LabelValuesRequest::kLabelName, query.label_name);
  }
};

}  // namespace PromPP::Prometheus::Remote