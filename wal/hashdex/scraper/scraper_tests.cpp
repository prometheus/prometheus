#include <gtest/gtest.h>

#include "scraper.h"
#include "wal/hashdex/metric.h"
#include "wal/hashdex/test_fixture.h"

#include "primitives/label_set.h"
#include "primitives/primitives.h"
#include "primitives/sample.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Primitives::Sample;
using PromPP::Primitives::Timestamp;
using PromPP::Prometheus::MetadataType;
using PromPP::WAL::hashdex::Metadata;
using PromPP::WAL::hashdex::Metric;
using PromPP::WAL::hashdex::scraper::Error;
using PromPP::WAL::hashdex::scraper::OpenMetricsScraper;
using PromPP::WAL::hashdex::scraper::PrometheusScraper;
using std::operator""sv;
using std::operator""s;

struct ScraperCase {
  std::string_view buffer;
  Error result;
  std::vector<Metadata> metadata{};
  std::vector<Metric> metrics{};
};

constexpr Timestamp kDefaultTimestamp = std::numeric_limits<Timestamp>::max();

template <class Scraper>
class ScraperFixture : public ::testing::TestWithParam<ScraperCase> {
 protected:
  Scraper scraper_;

  [[nodiscard]] std::vector<Metric> get_metrics() const noexcept { return PromPP::WAL::hashdex::get_metrics(scraper_.metrics()); }
  [[nodiscard]] std::vector<Metadata> get_metadata() const noexcept { return PromPP::WAL::hashdex::get_metadata(scraper_.metadata()); }
};

class PrometheusScraperFixture : public ScraperFixture<PrometheusScraper> {
 protected:
  void SetUp() override { calculate_labelset_hash(const_cast<ScraperCase&>(GetParam()).metrics); }
};

TEST_P(PrometheusScraperFixture, Test) {
  // Arrange
  std::string buffer(GetParam().buffer.data(), GetParam().buffer.size());
  buffer.shrink_to_fit();

  // Act
  const auto result = scraper_.parse(buffer, kDefaultTimestamp);
  const auto metrics = get_metrics();
  const auto metadata = get_metadata();

  // Assert
  EXPECT_EQ(GetParam().result, result);
  EXPECT_EQ(GetParam().metrics, metrics);
  EXPECT_EQ(GetParam().metadata, metadata);
}

INSTANTIATE_TEST_SUITE_P(EmptyBuffer, PrometheusScraperFixture, testing::Values(ScraperCase{.buffer = "", .result = Error::kNoError, .metrics = {}}));

INSTANTIATE_TEST_SUITE_P(Comment, PrometheusScraperFixture, testing::Values(ScraperCase{.buffer = "# comment\n", .result = Error::kNoError, .metrics = {}}));

INSTANTIATE_TEST_SUITE_P(HelpMetadata,
                         PrometheusScraperFixture,
                         testing::Values(ScraperCase{.buffer = "# HELP go_goroutines Number of goroutines that currently exist.\n",
                                                     .result = Error::kNoError,
                                                     .metadata = {Metadata{.metric_name = "go_goroutines",
                                                                           .text = "Number of goroutines that currently exist.",
                                                                           .type = MetadataType::kHelp}}},
                                         ScraperCase{.buffer = "# HELP go_goroutines\n",
                                                     .result = Error::kNoError,
                                                     .metadata = {Metadata{.metric_name = "go_goroutines", .text = "", .type = MetadataType::kHelp}}},
                                         ScraperCase{.buffer = "# HELP go_goroutines \t \n",
                                                     .result = Error::kNoError,
                                                     .metadata = {Metadata{.metric_name = "go_goroutines", .text = "", .type = MetadataType::kHelp}}}));

INSTANTIATE_TEST_SUITE_P(
    TypeMetadata,
    PrometheusScraperFixture,
    testing::Values(ScraperCase{.buffer = "# TYPE go_goroutines gauge\n",
                                .result = Error::kNoError,
                                .metadata = {Metadata{.metric_name = "go_goroutines", .text = "gauge", .type = MetadataType::kType}}},
                    ScraperCase{.buffer = "      # TYPE extended_monitoring_annotations gauge\n",
                                .result = Error::kNoError,
                                .metadata = {Metadata{.metric_name = "extended_monitoring_annotations", .text = "gauge", .type = MetadataType::kType}}}));

INSTANTIATE_TEST_SUITE_P(MetricNameInsideLabelSet,
                         PrometheusScraperFixture,
                         testing::Values(ScraperCase{.buffer = "{q=\"0.9\",\"http.status\",a=\"b\"} 8.3835e-05\n"sv,
                                                     .result = Error::kNoError,
                                                     .metrics = {Metric{.timeseries = {LabelViewSet{
                                                                                           {"q", "0.9"},
                                                                                           {"__name__", "http.status"},
                                                                                           {"a", "b"},
                                                                                       },
                                                                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}}}}));

INSTANTIATE_TEST_SUITE_P(NullByte,
                         PrometheusScraperFixture,
                         testing::Values(ScraperCase{.buffer = "null_byte_metric{a=\"abc\x00\"} 1\n"sv,
                                                     .result = Error::kNoError,
                                                     .metrics = {Metric{.timeseries = {LabelViewSet{
                                                                                           {"__name__", "null_byte_metric"},
                                                                                           {"a", "abc\x00"sv},
                                                                                       },
                                                                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}},
                                         ScraperCase{.buffer = "a{b=\"\x00ss\"} 1\n"sv,
                                                     .result = Error::kNoError,
                                                     .metrics = {Metric{.timeseries = {LabelViewSet{
                                                                                           {"__name__", "a"},
                                                                                           {"b", "\x00ss"sv},
                                                                                       },
                                                                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}},
                                         ScraperCase{.buffer = "a{b=\"\x00\"} 1\n"sv,
                                                     .result = Error::kNoError,
                                                     .metrics = {Metric{.timeseries = {LabelViewSet{
                                                                                           {"__name__", "a"},
                                                                                           {"b", "\x00"sv},
                                                                                       },
                                                                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}},
                                         ScraperCase{.buffer = "a{b=\x00\"ssss\"} 1\n"sv, .result = Error::kUnexpectedToken, .metrics = {}},
                                         ScraperCase{.buffer = "a{b=\"\x00\n"sv, .result = Error::kUnexpectedToken, .metrics = {}},
                                         ScraperCase{.buffer = "a{b\x00=\"hiih\"}	\n"sv, .result = Error::kUnexpectedToken, .metrics = {}},
                                         ScraperCase{.buffer = "a\x00{b=\"ddd\"} 1\n"sv, .result = Error::kInvalidValue, .metrics = {}},
                                         ScraperCase{.buffer = "a 0 1\x00\n"sv, .result = Error::kUnexpectedToken, .metrics = {}}));

INSTANTIATE_TEST_SUITE_P(
    InvalidInput,
    PrometheusScraperFixture,
    testing::Values(ScraperCase{.buffer = "a\n", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "a{b='c'} 1\n", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "a{b=\n", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "a{\xff=\"foo\"} 1\n", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "a{b=\"\xff\"} 1\n", .result = Error::kInvalidUtf8, .metrics = {}},
                    ScraperCase{.buffer = "{\"a\", \"b = \"c\"}\n", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "{\"a\",b\\nc=\"d\"} 1\n", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "a true\n", .result = Error::kInvalidValue, .metrics = {}},
                    ScraperCase{.buffer = "something_weird{problem=\"\n", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "empty_label_name{=\"\"} 0\n", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "empty_label_name{a=\"\", =\"2\"} 0\n", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "foo 1_2\n", .result = Error::kInvalidValue, .metrics = {}},
                    ScraperCase{.buffer = "foo 0x1p-3\n", .result = Error::kInvalidValue, .metrics = {}},
                    ScraperCase{.buffer = "foo 0x1P-3\n", .result = Error::kInvalidValue, .metrics = {}},
                    ScraperCase{.buffer = "foo 0 1_2\n", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "{a=\"ok\"} 1\n", .result = Error::kNoMetricName, .metrics = {}},
                    ScraperCase{.buffer = "\"aaaa\"\n", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "go_gc_duration_seconds_count 8437 9223372036854775808\n", .result = Error::kInvalidTimestamp, .metrics = {}},
                    ScraperCase{.buffer = "go_gc_duration_seconds_count \n", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "go_gc_duration_seconds_count\n", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "go_gc_duration_seconds_count", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "go_gc_duration_seconds_count ", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "{\"a\"\n}\n", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "{} 1\n", .result = Error::kNoMetricName, .metrics = {}},
                    ScraperCase{.buffer = "{\"\xff\"\n}\n", .result = Error::kInvalidUtf8, .metrics = {}},
                    ScraperCase{.buffer = "# TYPE #\n#EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "# HELP #\n#EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
                    ScraperCase{.buffer = "# HELP metric_name value\xff\n", .result = Error::kInvalidUtf8, .metrics = {}}));

INSTANTIATE_TEST_SUITE_P(Labels,
                         PrometheusScraperFixture,
                         testing::Values(ScraperCase{.buffer = "go_gc_duration_seconds_count 8437\n",
                                                     .result = Error::kNoError,
                                                     .metrics = {Metric{.timeseries = {LabelViewSet{{"__name__", "go_gc_duration_seconds_count"}},
                                                                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8437}}}}}},
                                         ScraperCase{.buffer = "example_metric{value=\"1\", z=\"\"} 0.144\n",
                                                     .result = Error::kNoError,
                                                     .metrics = {Metric{.timeseries = {LabelViewSet{{"__name__", "example_metric"}, {"value", "1"}},
                                                                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 0.144}}}}}},
                                         ScraperCase{.buffer = "go_gc_duration_seconds_count 8437 12345\n",
                                                     .result = Error::kNoError,
                                                     .metrics = {Metric{.timeseries = {LabelViewSet{{"__name__", "go_gc_duration_seconds_count"}},
                                                                                       BareBones::Vector<Sample>{Sample{12345, 8437}}}}}},
                                         ScraperCase{.buffer = "go_gc_duration_seconds_count{} 0\n",
                                                     .result = Error::kNoError,
                                                     .metrics = {Metric{.timeseries = {LabelViewSet{{"__name__", "go_gc_duration_seconds_count"}},
                                                                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 0}}}}}},
                                         ScraperCase{.buffer = "go_info{version=\"go1.23.1\"} 1\n",
                                                     .result = Error::kNoError,
                                                     .metrics = {Metric{.timeseries = {LabelViewSet{
                                                                                           {"__name__", "go_info"},
                                                                                           {"version", "go1.23.1"},
                                                                                       },
                                                                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}},
                                         ScraperCase{.buffer = "go_info{version=\"go1.23.1\", os=\"linux\"} 1\n",
                                                     .result = Error::kNoError,
                                                     .metrics = {Metric{.timeseries = {LabelViewSet{
                                                                                           {"__name__", "go_info"},
                                                                                           {"version", "go1.23.1"},
                                                                                           {"os", "linux"},
                                                                                       },
                                                                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}}));
INSTANTIATE_TEST_SUITE_P(
    PromTest,
    PrometheusScraperFixture,
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
        .result = Error::kNoError,
        .metadata = {Metadata{.metric_name = "go_gc_duration_seconds", .text = "A summary of the GC invocation durations.", .type = MetadataType::kHelp},
                     Metadata{.metric_name = "go_gc_duration_seconds", .text = "summary", .type = MetadataType::kType},
                     Metadata{.metric_name = "nohelp1", .text = "", .type = MetadataType::kHelp},
                     Metadata{.metric_name = "nohelp2", .text = "", .type = MetadataType::kHelp},
                     Metadata{.metric_name = "go_goroutines", .text = "Number of goroutines that currently exist.", .type = MetadataType::kHelp},
                     Metadata{.metric_name = "go_goroutines", .text = "gauge", .type = MetadataType::kType},
                     Metadata{.metric_name = "metric", .text = "foo\000bar"sv, .type = MetadataType::kHelp}},
        .metrics = {Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go_gc_duration_seconds"},
                                              {"quantile", "0"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 4.9351e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go_gc_duration_seconds"},
                                              {"quantile", "0.25"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 7.424100000000001e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go_gc_duration_seconds"},
                                              {"quantile", "0.5"},
                                              {"a", "b"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go_gc_duration_seconds"},
                                              {"quantile", "0.8"},
                                              {"a", "b"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go_gc_duration_seconds"},
                                              {"quantile", "0.9"},
                                              {"a", "b"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "wind_speed"},
                                              {"A", "2"},
                                              {"c", "3"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 12345}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go_gc_duration_seconds"},
                                              {"quantile", "1.0"},
                                              {"a", "b"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go_gc_duration_seconds"},
                                              {"quantile", "1.0"},
                                              {"a", "b"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go_gc_duration_seconds"},
                                              {"quantile", "1.0"},
                                              {"a", "b"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go_gc_duration_seconds"},
                                              {"quantile", "1.0"},
                                              {"a", "b"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go_gc_duration_seconds"},
                                              {"quantile", "2.0"},
                                              {"a", "b"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go_gc_duration_seconds_count"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 99}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "some:aggregate:rate5m"},
                                              {"a_b", "c"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go_goroutines"},
                                          },
                                          BareBones::Vector<Sample>{Sample{123123, 33}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "_metric_starting_with_underscore"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "testmetric"},
                                              {"_label_starting_with_underscore", "foo"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "testmetric"},
                                              {"label", "\"bar\""},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "null_byte_metric"},
                                              {"a", "abc\000"sv},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}}));

INSTANTIATE_TEST_SUITE_P(
    Utf8PromTest,
    PrometheusScraperFixture,
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
        .result = Error::kNoError,
        .metadata = {Metadata{.metric_name = "go.gc_duration_seconds", .text = "A summary of the GC invocation durations.", .type = MetadataType::kHelp},
                     Metadata{.metric_name = "go.gc_duration_seconds", .text = "summary", .type = MetadataType::kType}},
        .metrics = {Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go.gc_duration_seconds"},
                                              {"quantile", "0"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 4.9351e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go.gc_duration_seconds"},
                                              {"quantile", "0.25"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 7.424100000000001e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go.gc_duration_seconds"},
                                              {"quantile", "0.5"},
                                              {"a", "b"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go.gc_duration_seconds"},
                                              {"quantile", "0.8"},
                                              {"a", "b"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go.gc_duration_seconds"},
                                              {"quantile", "0.9"},
                                              {"a", "b"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go.gc_duration_seconds"},
                                              {"quantile", "1.0"},
                                              {"a", "b"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go.gc_duration_seconds"},
                                              {"quantile", "1.0"},
                                              {"a", "b"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go.gc_duration_seconds"},
                                              {"quantile", "1.0"},
                                              {"a", "b"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go.gc_duration_seconds"},
                                              {"quantile", "1.0"},
                                              {"a", "b"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
                    Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "go.gc_duration_seconds_count"},
                                          },
                                          BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 99}}}},
                    Metric{.timeseries = {
                               LabelViewSet{
                                   {"__name__",
                                    "\x48\x65\x69\x7a\xc3\xb6\x6c\x72\xc3\xbc\x63\x6b\x73\x74\x6f\xc3\x9f\x61\x62\x64\xc3\xa4\x6d\x70\x66\x75\x6e\x67\x20\x31"
                                    "\x30\xe2\x82\xac\x20\x6d\x65\x74\x72\x69\x63\x20\x77\x69\x74\x68\x20\x22\x69\x6e\x74\x65\x72\x65\x73\x74\x69\x6e\x67\x22"
                                    "\x20\x7b\x63\x68\x61\x72\x61\x63\x74\x65\x72\x0a\x63\x68\x6f\x69\x63\x65\x73\x7d"},
                                   {"\x73\x74\x72\x61\x6e\x67\x65\xc2\xa9\xe2\x84\xa2\x0a\x27\x71\x75\x6f\x74\x65\x64\x27\x20\x22\x6e\x61\x6d\x65\x22", "6"},
                               },
                               BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 10.0}}}}}}));

INSTANTIATE_TEST_SUITE_P(
    SpecificFloatValue,
    PrometheusScraperFixture,
    testing::Values(ScraperCase{
        .buffer = "test_nan nan\n"
                  "rest_client_exec_plugin_ttl_seconds Inf\n"
                  "rest_client_exec_plugin_ttl_seconds +Inf\n"
                  "rest_client_exec_plugin_ttl_seconds -Inf\n",
        .result = Error::kNoError,
        .metrics = {
            Metric{.timeseries = {LabelViewSet{{"__name__", "test_nan"}}, BareBones::Vector<Sample>{Sample{kDefaultTimestamp, PromPP::Prometheus::kNormalNan}}}},
            Metric{.timeseries = {LabelViewSet{{"__name__", "rest_client_exec_plugin_ttl_seconds"}},
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, std::numeric_limits<double>::infinity()}}}},
            Metric{.timeseries = {LabelViewSet{{"__name__", "rest_client_exec_plugin_ttl_seconds"}},
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, std::numeric_limits<double>::infinity()}}}},
            Metric{.timeseries = {LabelViewSet{{"__name__", "rest_client_exec_plugin_ttl_seconds"}},
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, -std::numeric_limits<double>::infinity()}}}}}}));

INSTANTIATE_TEST_SUITE_P(NoNewLine,
                         PrometheusScraperFixture,
                         testing::Values(ScraperCase{.buffer = "extended_monitoring_enabled{namespace=\"kube-system\"} 1",
                                                     .result = Error::kNoError,
                                                     .metrics = {Metric{.timeseries = {LabelViewSet{
                                                                                           {"__name__", "extended_monitoring_enabled"},
                                                                                           {"namespace", "kube-system"},
                                                                                       },
                                                                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}},
                                         ScraperCase{.buffer = "extended_monitoring_enabled{namespace=\"kube-system\"} 1 12345",
                                                     .result = Error::kNoError,
                                                     .metrics = {Metric{.timeseries = {LabelViewSet{
                                                                                           {"__name__", "extended_monitoring_enabled"},
                                                                                           {"namespace", "kube-system"},
                                                                                       },
                                                                                       BareBones::Vector<Sample>{Sample{12345, 1}}}}}}));

class OpenMetricsScraperFixture : public ScraperFixture<OpenMetricsScraper> {
 protected:
  void SetUp() override { calculate_labelset_hash(const_cast<ScraperCase&>(GetParam()).metrics); }
};

TEST_P(OpenMetricsScraperFixture, Test) {
  // Arrange
  std::string buffer(GetParam().buffer.data(), GetParam().buffer.size());
  buffer.shrink_to_fit();

  // Act
  const auto result = scraper_.parse(buffer, kDefaultTimestamp);
  const auto metrics = get_metrics();
  const auto metadata = get_metadata();

  // Assert
  EXPECT_EQ(GetParam().result, result);
  EXPECT_EQ(GetParam().metrics, metrics);
  EXPECT_EQ(GetParam().metadata, metadata);
}

INSTANTIATE_TEST_SUITE_P(EmptyBuffer,
                         OpenMetricsScraperFixture,
                         testing::Values(ScraperCase{.buffer = "# EOF", .result = Error::kNoError, .metrics = {}},
                                         ScraperCase{.buffer = "# EOF\n", .result = Error::kNoError, .metrics = {}}));

INSTANTIATE_TEST_SUITE_P(
    InvalidInput,
    OpenMetricsScraperFixture,
    testing::Values(
        ScraperCase{.buffer = "", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "metric", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "metric 1", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "metric 1\n",
                    .result = Error::kUnexpectedToken,
                    .metrics = {Metric{.timeseries = {LabelViewSet{{"__name__", "metric"}}, BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}},
        ScraperCase{.buffer = R"(metric_total 1 # {aa="bb"} 4)",
                    .result = Error::kUnexpectedToken,
                    .metrics = {Metric{.timeseries = {LabelViewSet{{"__name__", "metric_total"}}, BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}},
        ScraperCase{.buffer = "a\n#EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "\n\n#EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a 1\n#EOF\n",
                    .result = Error::kUnexpectedToken,
                    .metrics = {Metric{.timeseries = {LabelViewSet{{"__name__", "a"}}, BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}},
        ScraperCase{.buffer = "9\n#EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "# TYPE \n#EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "# UNIT \n#EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "# HELP \n#EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "# HELP m\n#EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a\t1\n# EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a\t1\n#EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a 1\t2\n# EOF\n", .result = Error::kInvalidValue, .metrics = {}},
        ScraperCase{.buffer = "a 1\t2\n#EOF\n", .result = Error::kInvalidValue, .metrics = {}},
        ScraperCase{.buffer = "a 1 2 \n#EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a 1 2 #\n#EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a 1 1z\n#EOF\n", .result = Error::kInvalidTimestamp, .metrics = {}},
        ScraperCase{.buffer = " # EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "#\tTYPE c counter\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a 1 1 1\n# EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a{b='c'} 1\n# EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a{,b=\"c\"} 1\n# EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a{b=\"c\"d=\"e\"} 1\n# EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a{b=\"c\",,d=\"e\"} 1\n# EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a{b=\n# EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a{\xff=\"foo\"} 1\n# EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a{b=\"\xff\"} 1\n# EOF\n", .result = Error::kInvalidUtf8, .metrics = {}},
        ScraperCase{.buffer = "{\"a\",\"b = \"c\"}\n# EOF", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "{\"a\",b\\nc=\"d\"} 1\n# EOF", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a true\n", .result = Error::kInvalidValue, .metrics = {}},
        ScraperCase{.buffer = "something_weird{problem=\"\n# EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "empty_label_name{=\"\"} 0\n# EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "foo 1_2\n\n# EOF\n", .result = Error::kInvalidValue, .metrics = {}},
        ScraperCase{.buffer = "foo 0x1p-3\n\n# EOF\n", .result = Error::kInvalidValue, .metrics = {}},
        ScraperCase{.buffer = "foo 0x1P-3\n\n# EOF\n", .result = Error::kInvalidValue, .metrics = {}},
        ScraperCase{.buffer = "foo 0 1_2\n\n# EOF\n", .result = Error::kInvalidTimestamp, .metrics = {}},
        ScraperCase{.buffer = "{b=\"c\",} 1\n# EOF", .result = Error::kNoMetricName, .metrics = {}},
        ScraperCase{.buffer = "a 1 NaN\n", .result = Error::kInvalidTimestamp, .metrics = {}},
        ScraperCase{.buffer = "a 1 -Inf\n", .result = Error::kInvalidTimestamp, .metrics = {}},
        ScraperCase{.buffer = "a 1 +Inf\n", .result = Error::kInvalidTimestamp, .metrics = {}},
        ScraperCase{.buffer = "a 1 Inf\n", .result = Error::kInvalidTimestamp, .metrics = {}},
        ScraperCase{.buffer = "a{b=\x00\"ssss\"} 1\n# EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a{b=\"\x00", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a{b\x00=\"hiih\"}	1", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "a\x00{b=\"ddd\"} 1", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "#", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "# comment\n# EOF", .result = Error::kUnexpectedToken, .metrics = {}}));

INSTANTIATE_TEST_SUITE_P(
    DISABLED_InvalidInputInExamplers,
    OpenMetricsScraperFixture,
    testing::Values(
        ScraperCase{.buffer = "custom_metric_total 1 # {aa=bb}\n# EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "custom_metric_total 1 # {aa=\"bb\"}\n# EOF\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "custom_metric_total 1 # {aa=\"bb\"}", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "custom_metric_total 1 # {bb}", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "custom_metric_total 1 # {bb, a=\"dd\"}", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "custom_metric_total 1 # {aa=\"bb\",,cc=\"dd\"} 1", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "custom_metric_total 1 # {aa=\"bb\"} 1_2", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "custom_metric_total 1 # {aa=\"bb\"} 0x1p-3", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "custom_metric_total 1 # {aa=\"bb\"} true", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "custom_metric_total 1 # {aa=\"bb\",cc=}", .result = Error::kUnexpectedToken, .metrics = {}},
        /*ScraperCase{.buffer = `custom_metric_total 1 # {aa=\"\xff\"} 9.0` .result = Error::kUnexpectedToken, .metrics = {}}*/
        ScraperCase{.buffer = "# TYPE hhh histogram\nhhh_bucket{le=\"+Inf\"} 1 # {aa=\"bb\"} 4 NaN\n", .result = Error::kInvalidTimestamp, .metrics = {}},
        ScraperCase{.buffer = "# TYPE hhh histogram\nhhh_bucket{le=\"+Inf\"} 1 # {aa=\"bb\"} 4 -Inf\n", .result = Error::kInvalidTimestamp, .metrics = {}},
        ScraperCase{.buffer = "# TYPE hhh histogram\nhhh_bucket{le=\"+Inf\"} 1 # {aa=\"bb\"} 4 Inf\n", .result = Error::kInvalidTimestamp, .metrics = {}},
        ScraperCase{.buffer = "custom_metric_total 1 # {b=\x00\"ssss\"} 1\n", .result = Error::kUnexpectedToken, .metrics = {}},
        ScraperCase{.buffer = "custom_metric_total 1 # {b=\"\x00ss\"} 1\n", .result = Error::kUnexpectedToken, .metrics = {}}));

INSTANTIATE_TEST_SUITE_P(
    PromTest,
    OpenMetricsScraperFixture,
    testing::Values(ScraperCase{
        .buffer = "# HELP go_gc_duration_seconds A summary of the GC invocation durations.\n"
                  "# TYPE go_gc_duration_seconds summary\n"
                  "# UNIT go_gc_duration_seconds seconds\n"
                  "go_gc_duration_seconds{quantile=\"0\"} 4.9351e-05\n"
                  "go_gc_duration_seconds{quantile=\"0.25\"} 7.424100000000001e-05\n"
                  "go_gc_duration_seconds{quantile=\"0.5\",a=\"b\"} 8.3835e-05\n"
                  "# HELP nohelp1 \n"
                  R"(# HELP help2 escape \ \n \\ \" \x chars)"
                  "\n"
                  "# UNIT nounit \n"
                  "go_gc_duration_seconds{quantile=\"1.0\",a=\"b\"} 8.3835e-05\n"
                  "go_gc_duration_seconds_count 99\n"
                  "some:aggregate:rate5m{a_b=\"c\"} 1\n"
                  "# HELP go_goroutines Number of goroutines that currently exist.\n"
                  "# TYPE go_goroutines gauge\n"
                  "go_goroutines 33 123.123\n"
                  "# TYPE hh histogram\n"
                  "hh_bucket{le=\"+Inf\"} 1\n"
                  "# TYPE gh gaugehistogram\n"
                  "gh_bucket{le=\"+Inf\"} 1\n"
                  "# TYPE hhh histogram\n"
                  "hhh_bucket{le=\"+Inf\"} 1 # {id=\"histogram-bucket-test\"} 4\n"
                  "hhh_count 1 # {id=\"histogram-count-test\"} 4\n"
                  "# TYPE ggh gaugehistogram\n"
                  "ggh_bucket{le=\"+Inf\"} 1 # {id=\"gaugehistogram-bucket-test\",xx=\"yy\"} 4 123.123\n"
                  "ggh_count 1 # {id=\"gaugehistogram-count-test\",xx=\"yy\"} 4 123.123\n"
                  "# TYPE smr_seconds summary\n"
                  "smr_seconds_count 2.0 # {id=\"summary-count-test\"} 1 123.321\n"
                  "smr_seconds_sum 42.0 # {id=\"summary-sum-test\"} 1 123.321\n"
                  "# TYPE ii info\n"
                  "ii{foo=\"bar\"} 1\n"
                  "# TYPE ss stateset\n"
                  "ss{ss=\"foo\"} 1\n"
                  "ss{ss=\"bar\"} 0\n"
                  "ss{A=\"a\"} 0\n"
                  "# TYPE un unknown\n"
                  "_metric_starting_with_underscore 1\n"
                  "testmetric{_label_starting_with_underscore=\"foo\"} 1\n"
                  R"(testmetric{label="\"bar\""} 1)"
                  "\n"
                  "# TYPE foo counter\n"
                  "foo_total 17.0 1520879607.789 # {id=\"counter-test\"} 5\n"
                  "# HELP metric foo\0bar\n"
                  "null_byte_metric{a=\"abc\x00\"} 1\n"
                  "# EOF\n"sv,
        .result = Error::kNoError,
        .metadata =
            {
                Metadata{.metric_name = "go_gc_duration_seconds", .text = "A summary of the GC invocation durations.", .type = MetadataType::kHelp},
                Metadata{.metric_name = "go_gc_duration_seconds", .text = "summary", .type = MetadataType::kType},
                Metadata{.metric_name = "go_gc_duration_seconds", .text = "seconds", .type = MetadataType::kUnit},
                Metadata{.metric_name = "nohelp1", .text = "", .type = MetadataType::kHelp},
                Metadata{.metric_name = "help2", .text = R"(escape \ \n \\ \" \x chars)", .type = MetadataType::kHelp},
                Metadata{.metric_name = "nounit", .text = "", .type = MetadataType::kUnit},
                Metadata{.metric_name = "go_goroutines", .text = "Number of goroutines that currently exist.", .type = MetadataType::kHelp},
                Metadata{.metric_name = "go_goroutines", .text = "gauge", .type = MetadataType::kType},
                Metadata{.metric_name = "hh", .text = "histogram", .type = MetadataType::kType},
                Metadata{.metric_name = "gh", .text = "gaugehistogram", .type = MetadataType::kType},
                Metadata{.metric_name = "hhh", .text = "histogram", .type = MetadataType::kType},
                Metadata{.metric_name = "ggh", .text = "gaugehistogram", .type = MetadataType::kType},
                Metadata{.metric_name = "smr_seconds", .text = "summary", .type = MetadataType::kType},
                Metadata{.metric_name = "ii", .text = "info", .type = MetadataType::kType},
                Metadata{.metric_name = "ss", .text = "stateset", .type = MetadataType::kType},
                Metadata{.metric_name = "un", .text = "unknown", .type = MetadataType::kType},
                Metadata{.metric_name = "foo", .text = "counter", .type = MetadataType::kType},
                Metadata{.metric_name = "metric", .text = "foo\0bar"sv, .type = MetadataType::kHelp},
            },
        .metrics = {
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "go_gc_duration_seconds"},
                                      {"quantile", "0"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 4.9351e-05}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "go_gc_duration_seconds"},
                                      {"quantile", "0.25"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 7.424100000000001e-05}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "go_gc_duration_seconds"},
                                      {"quantile", "0.5"},
                                      {"a", "b"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "go_gc_duration_seconds"},
                                      {"quantile", "1.0"},
                                      {"a", "b"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "go_gc_duration_seconds_count"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 99}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "some:aggregate:rate5m"},
                                      {"a_b", "c"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "go_goroutines"},
                                  },
                                  BareBones::Vector<Sample>{Sample{123123, 33}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "hh_bucket"},
                                      {"le", "+Inf"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "gh_bucket"},
                                      {"le", "+Inf"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "hhh_bucket"},
                                      {"le", "+Inf"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "hhh_count"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "ggh_bucket"},
                                      {"le", "+Inf"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "ggh_count"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "smr_seconds_count"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 2}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "smr_seconds_sum"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 42}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "ii"},
                                      {"foo", "bar"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "ss"},
                                      {"ss", "foo"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "ss"},
                                      {"ss", "bar"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 0}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "ss"},
                                      {"A", "a"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 0}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "_metric_starting_with_underscore"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "testmetric"},
                                      {"_label_starting_with_underscore", "foo"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "testmetric"},
                                      {"label", "\"bar\""},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "foo_total"},
                                  },
                                  BareBones::Vector<Sample>{Sample{1520879607789, 17.0}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "null_byte_metric"},
                                      {"a", "abc\0"sv},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}},
        }}));

INSTANTIATE_TEST_SUITE_P(
    Utf8PromTest,
    OpenMetricsScraperFixture,
    testing::Values(ScraperCase{
        .buffer =
            "# HELP \"go.gc_duration_seconds\" A summary of the GC invocation durations.\n"
            "# TYPE \"go.gc_duration_seconds\" summary\n"
            "{\"go.gc_duration_seconds\",quantile=\"0\"} 4.9351e-05\n"
            "{\"go.gc_duration_seconds\",quantile=\"0.25\"} 7.424100000000001e-05\n"
            "{\"go.gc_duration_seconds\",quantile=\"0.5\",a=\"b\"} 8.3835e-05\n"
            "{\"http.status\",q=\"0.9\",a=\"b\"} 8.3835e-05\n"
            "{q=\"0.9\",\"http.status\",a=\"b\"} 8.3835e-05\n"
            "{\"go.gc_duration_seconds_sum\"} 0.004304266\n"
            "{\"\x48\x65\x69\x7a\xc3\xb6\x6c\x72\xc3\xbc\x63\x6b\x73\x74\x6f\xc3\x9f\x61\x62\x64\xc3\xa4\x6d\x70\x66\x75\x6e\x67\x20"
            "\x31\x30\xe2\x82\xac\x20\x6d\x65\x74\x72\x69\x63\x20\x77\x69\x74\x68\x20\x5c\x22\x69\x6e\x74\x65\x72\x65\x73\x74\x69"
            "\x6e\x67\x5c\x22\x20\x7b\x63\x68\x61\x72\x61\x63\x74\x65\x72\x5c\x6e\x63\x68\x6f\x69\x63\x65\x73\x7d\","
            "\"\x73\x74\x72\x61\x6e\x67\x65\xc2\xa9\xe2\x84\xa2\x5c\x6e\x27\x71\x75\x6f\x74\x65\x64\x27\x20\x5c\x22\x6e\x61\x6d\x65\x5c\x22\"=\"6\"} 10.0\n"
            "# EOF\n"sv,
        .result = Error::kNoError,
        .metadata =
            {
                Metadata{.metric_name = "go.gc_duration_seconds", .text = "A summary of the GC invocation durations.", .type = MetadataType::kHelp},
                Metadata{.metric_name = "go.gc_duration_seconds", .text = "summary", .type = MetadataType::kType},
            },
        .metrics = {
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "go.gc_duration_seconds"},
                                      {"quantile", "0"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 4.9351e-05}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "go.gc_duration_seconds"},
                                      {"quantile", "0.25"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 7.424100000000001e-05}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "go.gc_duration_seconds"},
                                      {"quantile", "0.5"},
                                      {"a", "b"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "http.status"},
                                      {"q", "0.9"},
                                      {"a", "b"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"q", "0.9"},
                                      {"__name__", "http.status"},
                                      {"a", "b"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 8.3835e-05}}}},
            Metric{.timeseries = {LabelViewSet{
                                      {"__name__", "go.gc_duration_seconds_sum"},
                                  },
                                  BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 0.004304266}}}},
            Metric{
                .timeseries = {LabelViewSet{
                                   {"__name__",
                                    "\x48\x65\x69\x7a\xc3\xb6\x6c\x72\xc3\xbc\x63\x6b\x73\x74\x6f\xc3\x9f\x61\x62\x64\xc3\xa4\x6d\x70\x66\x75\x6e\x67\x20\x31"
                                    "\x30\xe2\x82\xac\x20\x6d\x65\x74\x72\x69\x63\x20\x77\x69\x74\x68\x20\x22\x69\x6e\x74\x65\x72\x65\x73\x74\x69\x6e\x67\x22"
                                    "\x20\x7b\x63\x68\x61\x72\x61\x63\x74\x65\x72\x0a\x63\x68\x6f\x69\x63\x65\x73\x7d"},
                                   {"\x73\x74\x72\x61\x6e\x67\x65\xc2\xa9\xe2\x84\xa2\x0a\x27\x71\x75\x6f\x74\x65\x64\x27\x20\x22\x6e\x61\x6d\x65\x22", "6"},
                               },
                               BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 10.0}}}},
        }}));

INSTANTIATE_TEST_SUITE_P(NullByte,
                         OpenMetricsScraperFixture,
                         testing::Values(ScraperCase{.buffer = "null_byte_metric{a=\"abc\x00\"} 1\n# EOF\n"sv,
                                                     .result = Error::kNoError,
                                                     .metrics = {Metric{.timeseries = {LabelViewSet{
                                                                                           {"__name__", "null_byte_metric"},
                                                                                           {"a", "abc\x00"sv},
                                                                                       },
                                                                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}},
                                         ScraperCase{.buffer = "null_byte_metric{a=\"abc\x00\"} 1\n# EOF\n"sv,
                                                     .result = Error::kNoError,
                                                     .metrics = {Metric{.timeseries = {LabelViewSet{
                                                                                           {"__name__", "null_byte_metric"},
                                                                                           {"a", "abc\x00"sv},
                                                                                       },
                                                                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}},
                                         ScraperCase{.buffer = "a{b=\"\x00\"} 1\n# EOF\n"sv,
                                                     .result = Error::kNoError,
                                                     .metrics = {Metric{.timeseries = {LabelViewSet{
                                                                                           {"__name__", "a"},
                                                                                           {"b", "\x00"sv},
                                                                                       },
                                                                                       BareBones::Vector<Sample>{Sample{kDefaultTimestamp, 1}}}}}},
                                         ScraperCase{.buffer = "a{b=\x00\"ssss\"} 1\n# EOF\n"sv, .result = Error::kUnexpectedToken, .metrics = {}}));

}  // namespace
