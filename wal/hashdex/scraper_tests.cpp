#include <gtest/gtest.h>

#include "scraper.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Primitives::Sample;
using PromPP::Primitives::TimeseriesSemiview;
using PromPP::Primitives::Timestamp;
using PromPP::WAL::hashdex::Scraper;
using std::operator""sv;
using std::operator""s;

struct ScrapedItem {
  TimeseriesSemiview timeseries{};
  uint64_t hash{};

  bool operator==(const ScrapedItem&) const noexcept = default;
};

std::ostream& operator<<(std::ostream& stream, const ScrapedItem& item) {
  stream << "hash: " << item.hash << ", labels: { ";

  for (const auto& [name, value] : item.timeseries.label_set()) {
    stream << name << " = " << value << ", ";
  }

  stream << " }, samples: { ";

  for (const auto& [timestamp, value] : item.timeseries.samples()) {
    stream << timestamp << " => " << value << ", ";
  }

  stream << " }";

  return stream;
}

struct ScraperCase {
  std::string_view buffer;
  Scraper::Error result;
  std::vector<ScrapedItem> items;
};

constexpr Timestamp kDefaultTimestamp = std::numeric_limits<Timestamp>::max();

class ScraperFixture : public ::testing::TestWithParam<ScraperCase> {
 protected:
  static constexpr PromPP::Primitives::LabelView kTargetIdLabel = {"__target_id__", "1"};

  Scraper scraper_;

  void SetUp() override {
    for (auto& item : const_cast<ScraperCase&>(GetParam()).items) {
      item.timeseries.label_set().add(kTargetIdLabel);
      item.hash = PromPP::Primitives::hash::hash_of_label_set(item.timeseries.label_set());
    }
  }

  [[nodiscard]] std::vector<ScrapedItem> get_items() const noexcept {
    std::vector<ScrapedItem> items;
    items.reserve(scraper_.size());

    for (auto& item : scraper_) {
      auto& scraped_item = items.emplace_back(ScrapedItem{.hash = item.hash()});
      item.read(scraped_item.timeseries);
    }

    return items;
  }
};

TEST_P(ScraperFixture, Test) {
  // Arrange
  std::string buffer(GetParam().buffer.data(), GetParam().buffer.size());

  // Act
  const auto result = scraper_.parse(buffer, kDefaultTimestamp, kTargetIdLabel.second);
  const auto items = get_items();

  // Assert
  EXPECT_EQ(GetParam().result, result);
  EXPECT_EQ(GetParam().items, items);
}

INSTANTIATE_TEST_SUITE_P(EmptyBuffer, ScraperFixture, testing::Values(ScraperCase{.buffer = "", .result = Scraper::Error::kNoError, .items = {}}));

INSTANTIATE_TEST_SUITE_P(Comment, ScraperFixture, testing::Values(ScraperCase{.buffer = "# comment\n", .result = Scraper::Error::kNoError, .items = {}}));

INSTANTIATE_TEST_SUITE_P(HelpMetadata,
                         ScraperFixture,
                         testing::Values(ScraperCase{.buffer = "# HELP go_goroutines Number of goroutines that currently exist.\n",
                                                     .result = Scraper::Error::kNoError,
                                                     .items = {}}));

INSTANTIATE_TEST_SUITE_P(
    TypeMetadata,
    ScraperFixture,
    testing::Values(ScraperCase{.buffer = "# TYPE go_goroutines gauge\n", .result = Scraper::Error::kNoError, .items = {}},
                    ScraperCase{.buffer = "      # TYPE extended_monitoring_annotations gauge\n", .result = Scraper::Error::kNoError, .items = {}}));

INSTANTIATE_TEST_SUITE_P(NullByte,
                         ScraperFixture,
                         testing::Values(ScraperCase{.buffer = "null_byte_metric{a=\"abc\x00\"} 1\n"sv,
                                                     .result = Scraper::Error::kNoError,
                                                     .items = {ScrapedItem{.timeseries = {LabelViewSet{
                                                                                              {"__name__", "null_byte_metric"},
                                                                                              {"a", "abc\x00"sv},
                                                                                          },
                                                                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}},
                                         ScraperCase{.buffer = "a{b=\"\x00ss\"} 1\n"sv,
                                                     .result = Scraper::Error::kNoError,
                                                     .items = {ScrapedItem{.timeseries = {LabelViewSet{
                                                                                              {"__name__", "a"},
                                                                                              {"b", "\x00ss"sv},
                                                                                          },
                                                                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}},
                                         ScraperCase{.buffer = "a{b=\"\x00\"} 1\n"sv,
                                                     .result = Scraper::Error::kNoError,
                                                     .items = {ScrapedItem{.timeseries = {LabelViewSet{
                                                                                              {"__name__", "a"},
                                                                                              {"b", "\x00"sv},
                                                                                          },
                                                                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}},
                                         ScraperCase{.buffer = "a{b=\x00\"ssss\"} 1\n"sv, .result = Scraper::Error::kUnexpectedToken, .items = {}},
                                         ScraperCase{.buffer = "a{b=\"\x00\n"sv, .result = Scraper::Error::kUnexpectedToken, .items = {}},
                                         ScraperCase{.buffer = "a{b\x00=\"hiih\"}	\n"sv, .result = Scraper::Error::kUnexpectedToken, .items = {}},
                                         ScraperCase{.buffer = "a\x00{b=\"ddd\"} 1\n"sv, .result = Scraper::Error::kUnexpectedToken, .items = {}},
                                         ScraperCase{.buffer = "a 0 1\x00\n"sv, .result = Scraper::Error::kUnexpectedToken, .items = {}}));

INSTANTIATE_TEST_SUITE_P(
    InvalidInput,
    ScraperFixture,
    testing::Values(ScraperCase{.buffer = "a\n", .result = Scraper::Error::kUnexpectedToken, .items = {}},
                    ScraperCase{.buffer = "a{b='c'} 1\n", .result = Scraper::Error::kUnexpectedToken, .items = {}},
                    ScraperCase{.buffer = "a{b=\n", .result = Scraper::Error::kUnexpectedToken, .items = {}},
                    ScraperCase{.buffer = "a{\xff=\"foo\"} 1\n", .result = Scraper::Error::kUnexpectedToken, .items = {}},
                    ScraperCase{.buffer = "a{b=\"\xff\"} 1\n", .result = Scraper::Error::kInvalidUtf8, .items = {}},
                    ScraperCase{.buffer = "{\"a\", \"b = \"c\"}\n", .result = Scraper::Error::kUnexpectedToken, .items = {}},
                    ScraperCase{.buffer = "{\"a\",b\\nc=\"d\"} 1\n", .result = Scraper::Error::kUnexpectedToken, .items = {}},
                    ScraperCase{.buffer = "a true\n", .result = Scraper::Error::kInvalidValue, .items = {}},
                    ScraperCase{.buffer = "something_weird{problem=\"\n", .result = Scraper::Error::kUnexpectedToken, .items = {}},
                    ScraperCase{.buffer = "empty_label_name{=\"\"} 0\n", .result = Scraper::Error::kUnexpectedToken, .items = {}},
                    ScraperCase{.buffer = "empty_label_name{a=\"\", =\"2\"} 0\n", .result = Scraper::Error::kUnexpectedToken, .items = {}},
                    ScraperCase{.buffer = "foo 1_2\n", .result = Scraper::Error::kInvalidValue, .items = {}},
                    ScraperCase{.buffer = "foo 0x1p-3\n", .result = Scraper::Error::kInvalidValue, .items = {}},
                    ScraperCase{.buffer = "foo 0x1P-3\n", .result = Scraper::Error::kInvalidValue, .items = {}},
                    ScraperCase{.buffer = "foo 0 1_2\n", .result = Scraper::Error::kUnexpectedToken, .items = {}},
                    ScraperCase{.buffer = "{a=\"ok\"} 1\n", .result = Scraper::Error::kNoMetricName, .items = {}},
                    ScraperCase{.buffer = "\"aaaa\"\n", .result = Scraper::Error::kUnexpectedToken, .items = {}},
                    ScraperCase{.buffer = "go_gc_duration_seconds_count 8437 9223372036854775808\n", .result = Scraper::Error::kInvalidTimestamp, .items = {}},
                    ScraperCase{.buffer = "go_gc_duration_seconds_count \n", .result = Scraper::Error::kUnexpectedToken, .items = {}},
                    ScraperCase{.buffer = "go_gc_duration_seconds_count\n", .result = Scraper::Error::kUnexpectedToken, .items = {}},
                    ScraperCase{.buffer = "go_gc_duration_seconds_count", .result = Scraper::Error::kNoError, .items = {}},
                    ScraperCase{.buffer = "go_gc_duration_seconds_count ", .result = Scraper::Error::kUnexpectedToken, .items = {}},
                    ScraperCase{.buffer = "{\"a\"\n}\n", .result = Scraper::Error::kUnexpectedToken, .items = {}},
                    ScraperCase{.buffer = "{\"\xff\"\n}\n", .result = Scraper::Error::kInvalidUtf8, .items = {}}));

INSTANTIATE_TEST_SUITE_P(Labels,
                         ScraperFixture,
                         testing::Values(ScraperCase{.buffer = "go_gc_duration_seconds_count 8437\n",
                                                     .result = Scraper::Error::kNoError,
                                                     .items = {ScrapedItem{.timeseries = {LabelViewSet{{"__name__", "go_gc_duration_seconds_count"}},
                                                                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8437}}}}}},
                                         ScraperCase{.buffer = "go_gc_duration_seconds_count 8437 12345\n",
                                                     .result = Scraper::Error::kNoError,
                                                     .items = {ScrapedItem{.timeseries = {LabelViewSet{{"__name__", "go_gc_duration_seconds_count"}},
                                                                                          BareBones::Vector<Sample>{Sample{12345, 8437}}}}}},
                                         ScraperCase{.buffer = "go_gc_duration_seconds_count{} 0\n",
                                                     .result = Scraper::Error::kNoError,
                                                     .items = {ScrapedItem{.timeseries = {LabelViewSet{{"__name__", "go_gc_duration_seconds_count"}},
                                                                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 0}}}}}},
                                         ScraperCase{.buffer = "go_info{version=\"go1.23.1\"} 1\n",
                                                     .result = Scraper::Error::kNoError,
                                                     .items = {ScrapedItem{.timeseries = {LabelViewSet{
                                                                                              {"__name__", "go_info"},
                                                                                              {"version", "go1.23.1"},
                                                                                          },
                                                                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}},
                                         ScraperCase{.buffer = "go_info{version=\"go1.23.1\", os=\"linux\"} 1\n",
                                                     .result = Scraper::Error::kNoError,
                                                     .items = {ScrapedItem{.timeseries = {LabelViewSet{
                                                                                              {"__name__", "go_info"},
                                                                                              {"version", "go1.23.1"},
                                                                                              {"os", "linux"},
                                                                                          },
                                                                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}}));
INSTANTIATE_TEST_SUITE_P(PromTest,
                         ScraperFixture,
                         testing::Values(ScraperCase{
                             .buffer = "# HELP go_gc_duration_seconds A summary of the GC invocation durations.\n"
                                       "# 	TYPE go_gc_duration_seconds summary\n"
                                       "go_gc_duration_seconds{quantile=\"0\"} 4.9351e-05\n"
                                       "go_gc_duration_seconds{quantile=\"0.25\",} 7.424100000000001e-05\n"
                                       "go_gc_duration_seconds{quantile=\"0.5\",a=\"b\"} 8.3835e-05\n"
                                       "go_gc_duration_seconds{quantile=\"0.8\", a=\"b\"} 8.3835e-05\n"
                                       "go_gc_duration_seconds{ quantile=\"0.9\", a=\"b\"} 8.3835e-05\n"
                                       "# Hrandom comment starting with prefix of HELP\n"
                                       "#\n"
                                       "wind_speed{A=\"2\",c=\"3\"} 12345\n"
                                       "# comment with escaped \\n newline\n"
                                       "# comment with escaped \\ escape character\n"
                                       "# HELP nohelp1\n"
                                       "# HELP nohelp2\n"
                                       "go_gc_duration_seconds{ quantile=\"1.0\", a=\"b\" } 8.3835e-05\n"
                                       "go_gc_duration_seconds { quantile=\"1.0\", a=\"b\" } 8.3835e-05\n"
                                       "go_gc_duration_seconds { quantile= \"1.0\", a= \"b\", } 8.3835e-05\n"
                                       "go_gc_duration_seconds { quantile = \"1.0\", a = \"b\" } 8.3835e-05\n"
                                       "go_gc_duration_seconds { quantile = \"2.0\" a = \"b\" } 8.3835e-05\n"
                                       "go_gc_duration_seconds_count 99\n"
                                       "some:aggregate:rate5m{a_b=\"c\"}	1\n"
                                       "# HELP go_goroutines Number of goroutines that currently exist.\n"
                                       "# TYPE go_goroutines gauge\n"
                                       "go_goroutines 33  	123123\n"
                                       "_metric_starting_with_underscore 1\n"
                                       "testmetric{_label_starting_with_underscore=\"foo\"} 1\n"
                                       R"(testmetric{label="\"bar\""} 1)"
                                       "\n"
                                       "# HELP metric foo\000bar\n"
                                       "null_byte_metric{a=\"abc\000\"} 1\n"sv,
                             .result = Scraper::Error::kNoError,
                             .items = {ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "go_gc_duration_seconds"},
                                                                      {"quantile", "0"},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 4.9351e-05}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "go_gc_duration_seconds"},
                                                                      {"quantile", "0.25"},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 7.424100000000001e-05}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "go_gc_duration_seconds"},
                                                                      {"quantile", "0.5"},
                                                                      {"a", "b"},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "go_gc_duration_seconds"},
                                                                      {"quantile", "0.8"},
                                                                      {"a", "b"},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "go_gc_duration_seconds"},
                                                                      {"quantile", "0.9"},
                                                                      {"a", "b"},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "wind_speed"},
                                                                      {"A", "2"},
                                                                      {"c", "3"},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 12345}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "go_gc_duration_seconds"},
                                                                      {"quantile", "1.0"},
                                                                      {"a", "b"},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "go_gc_duration_seconds"},
                                                                      {"quantile", "1.0"},
                                                                      {"a", "b"},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "go_gc_duration_seconds"},
                                                                      {"quantile", "1.0"},
                                                                      {"a", "b"},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "go_gc_duration_seconds"},
                                                                      {"quantile", "1.0"},
                                                                      {"a", "b"},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "go_gc_duration_seconds"},
                                                                      {"quantile", "2.0"},
                                                                      {"a", "b"},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "go_gc_duration_seconds_count"},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 99}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "some:aggregate:rate5m"},
                                                                      {"a_b", "c"},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "go_goroutines"},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{123123, 33}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "_metric_starting_with_underscore"},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "testmetric"},
                                                                      {"_label_starting_with_underscore", "foo"},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "testmetric"},
                                                                      {"label", "\"bar\""},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
                                       ScrapedItem{.timeseries = {LabelViewSet{
                                                                      {"__name__", "null_byte_metric"},
                                                                      {"a", "abc\000"sv},
                                                                  },
                                                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}}));

INSTANTIATE_TEST_SUITE_P(
    Utf8PromTest,
    ScraperFixture,
    testing::Values(ScraperCase{
        .buffer =
            "# HELP \"go.gc_duration_seconds\" A summary of the GC invocation durations.\n"
            "# 	TYPE \"go.gc_duration_seconds\" summary\n"
            "{\"go.gc_duration_seconds\",quantile=\"0\"} 4.9351e-05\n"
            "{\"go.gc_duration_seconds\",quantile=\"0.25\",} 7.424100000000001e-05\n"
            "{\"go.gc_duration_seconds\",quantile=\"0.5\",a=\"b\"} 8.3835e-05\n"
            "{\"go.gc_duration_seconds\",quantile=\"0.8\", a=\"b\"} 8.3835e-05\n"
            "{\"go.gc_duration_seconds\", quantile=\"0.9\", a=\"b\"} 8.3835e-05\n"
            "{\"go.gc_duration_seconds\", quantile=\"1.0\", a=\"b\" } 8.3835e-05\n"
            "{ \"go.gc_duration_seconds\", quantile=\"1.0\", a=\"b\" } 8.3835e-05\n"
            "{ \"go.gc_duration_seconds\", quantile= \"1.0\", a= \"b\", } 8.3835e-05\n"
            "{ \"go.gc_duration_seconds\", quantile = \"1.0\", a = \"b\" } 8.3835e-05\n"
            "{\"go.gc_duration_seconds_count\"} 99\n"
            "{\"\x48\x65\x69\x7a\xc3\xb6\x6c\x72\xc3\xbc\x63\x6b\x73\x74\x6f\xc3\x9f\x61\x62\x64\xc3\xa4\x6d\x70\x66\x75\x6e\x67\x20"
            "\x31\x30\xe2\x82\xac\x20\x6d\x65\x74\x72\x69\x63\x20\x77\x69\x74\x68\x20\x5c\x22\x69\x6e\x74\x65\x72\x65\x73\x74\x69"
            "\x6e\x67\x5c\x22\x20\x7b\x63\x68\x61\x72\x61\x63\x74\x65\x72\x5c\x6e\x63\x68\x6f\x69\x63\x65\x73\x7d\", "
            "\"\x73\x74\x72\x61\x6e\x67\x65\xc2\xa9\xe2\x84\xa2\x5c\x6e\x27\x71\x75\x6f\x74\x65\x64\x27\x20\x5c\x22\x6e\x61\x6d\x65\x5c\x22\"=\"6\"} 10.0\n",
        .result = Scraper::Error::kNoError,
        .items = {
            ScrapedItem{.timeseries = {LabelViewSet{
                                           {"__name__", "go.gc_duration_seconds"},
                                           {"quantile", "0"},
                                       },
                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 4.9351e-05}}}},
            ScrapedItem{.timeseries = {LabelViewSet{
                                           {"__name__", "go.gc_duration_seconds"},
                                           {"quantile", "0.25"},
                                       },
                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 7.424100000000001e-05}}}},
            ScrapedItem{.timeseries = {LabelViewSet{
                                           {"__name__", "go.gc_duration_seconds"},
                                           {"quantile", "0.5"},
                                           {"a", "b"},
                                       },
                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
            ScrapedItem{.timeseries = {LabelViewSet{
                                           {"__name__", "go.gc_duration_seconds"},
                                           {"quantile", "0.8"},
                                           {"a", "b"},
                                       },
                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
            ScrapedItem{.timeseries = {LabelViewSet{
                                           {"__name__", "go.gc_duration_seconds"},
                                           {"quantile", "0.9"},
                                           {"a", "b"},
                                       },
                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
            ScrapedItem{.timeseries = {LabelViewSet{
                                           {"__name__", "go.gc_duration_seconds"},
                                           {"quantile", "1.0"},
                                           {"a", "b"},
                                       },
                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
            ScrapedItem{.timeseries = {LabelViewSet{
                                           {"__name__", "go.gc_duration_seconds"},
                                           {"quantile", "1.0"},
                                           {"a", "b"},
                                       },
                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
            ScrapedItem{.timeseries = {LabelViewSet{
                                           {"__name__", "go.gc_duration_seconds"},
                                           {"quantile", "1.0"},
                                           {"a", "b"},
                                       },
                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
            ScrapedItem{.timeseries = {LabelViewSet{
                                           {"__name__", "go.gc_duration_seconds"},
                                           {"quantile", "1.0"},
                                           {"a", "b"},
                                       },
                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
            ScrapedItem{.timeseries = {LabelViewSet{
                                           {"__name__", "go.gc_duration_seconds_count"},
                                       },
                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 99}}}},
            ScrapedItem{
                .timeseries = {LabelViewSet{
                                   {"__name__",
                                    "\x48\x65\x69\x7a\xc3\xb6\x6c\x72\xc3\xbc\x63\x6b\x73\x74\x6f\xc3\x9f\x61\x62\x64\xc3\xa4\x6d\x70\x66\x75\x6e\x67\x20\x31"
                                    "\x30\xe2\x82\xac\x20\x6d\x65\x74\x72\x69\x63\x20\x77\x69\x74\x68\x20\x22\x69\x6e\x74\x65\x72\x65\x73\x74\x69\x6e\x67\x22"
                                    "\x20\x7b\x63\x68\x61\x72\x61\x63\x74\x65\x72\x0a\x63\x68\x6f\x69\x63\x65\x73\x7d"},
                                   {"\x73\x74\x72\x61\x6e\x67\x65\xc2\xa9\xe2\x84\xa2\x0a\x27\x71\x75\x6f\x74\x65\x64\x27\x20\x22\x6e\x61\x6d\x65\x22", "6"},
                               },
                               BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 10.0}}}}}}));

INSTANTIATE_TEST_SUITE_P(
    SpecificFloatValue,
    ScraperFixture,
    testing::Values(ScraperCase{
        .buffer = "test_nan nan\n"
                  "rest_client_exec_plugin_ttl_seconds Inf\n"
                  "rest_client_exec_plugin_ttl_seconds +Inf\n"
                  "rest_client_exec_plugin_ttl_seconds -Inf\n",
        .result = Scraper::Error::kNoError,
        .items = {ScrapedItem{.timeseries = {LabelViewSet{{"__name__", "test_nan"}},
                                             BareBones::Vector<Sample>{Sample{kDefaultTimestamp, PromPP::Prometheus::kNormalNan}}}},
                  ScrapedItem{.timeseries = {LabelViewSet{{"__name__", "rest_client_exec_plugin_ttl_seconds"}},
                                             BareBones::Vector<Sample>{Sample{kDefaultTimestamp, std::numeric_limits<double>::infinity()}}}},
                  ScrapedItem{.timeseries = {LabelViewSet{{"__name__", "rest_client_exec_plugin_ttl_seconds"}},
                                             BareBones::Vector<Sample>{Sample{kDefaultTimestamp, std::numeric_limits<double>::infinity()}}}},
                  ScrapedItem{.timeseries = {LabelViewSet{{"__name__", "rest_client_exec_plugin_ttl_seconds"}},
                                             BareBones::Vector<Sample>{Sample{kDefaultTimestamp, -std::numeric_limits<double>::infinity()}}}}}}));

INSTANTIATE_TEST_SUITE_P(NoNewLine,
                         ScraperFixture,
                         testing::Values(ScraperCase{.buffer = "extended_monitoring_enabled{namespace=\"kube-system\"} 1",
                                                     .result = Scraper::Error::kNoError,
                                                     .items = {ScrapedItem{.timeseries = {LabelViewSet{
                                                                                              {"__name__", "extended_monitoring_enabled"},
                                                                                              {"namespace", "kube-system"},
                                                                                          },
                                                                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}},
                                         ScraperCase{.buffer = "extended_monitoring_enabled{namespace=\"kube-system\"} 1 12345",
                                                     .result = Scraper::Error::kNoError,
                                                     .items = {ScrapedItem{.timeseries = {LabelViewSet{
                                                                                              {"__name__", "extended_monitoring_enabled"},
                                                                                              {"namespace", "kube-system"},
                                                                                          },
                                                                                          BareBones::Vector<Sample>{Sample{12345, 1}}}}}}));

}  // namespace