#include <gtest/gtest.h>

#include "metric.h"
#include "protobuf.h"
#include "wal/hashdex/test_fixture.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Primitives::Sample;
using PromPP::Prometheus::MetadataType;
using PromPP::WAL::hashdex::Metadata;
using PromPP::WAL::hashdex::Metric;
using std::operator""sv;

struct ProtobufCase {
  std::string_view protobuf;
  std::vector<Metadata> metadata{};
  std::vector<Metric> metrics{};
};

class ProtobufFixture : public ::testing::TestWithParam<ProtobufCase> {
 protected:
  PromPP::WAL::hashdex::Protobuf hashdex_;

  void SetUp() override { calculate_labelset_hash(const_cast<ProtobufCase&>(GetParam()).metrics); }
};

TEST_P(ProtobufFixture, Test) {
  // Arrange

  // Act
  hashdex_.presharding(GetParam().protobuf);
  const auto metrics = get_metrics(hashdex_);
  const auto metadata = get_metadata(hashdex_.metadata());

  // Assert
  EXPECT_EQ(GetParam().metrics, metrics);
  EXPECT_EQ(GetParam().metadata, metadata);
}

INSTANTIATE_TEST_SUITE_P(
    Tests,
    ProtobufFixture,
    testing::Values(ProtobufCase{
        .protobuf =
            "\x0a\x58\x0a\x10\x0a\x08\x5f\x5f\x6e\x61\x6d\x65\x5f\x5f\x12\x04\x74\x65\x73\x74\x0a\x14\x0a\x07\x63\x6c\x75\x73\x74\x65\x72\x12\x09\x63\x6c\x75"
            "\x73\x74\x65\x72\x2d\x30\x0a\x18\x0a\x0b\x5f\x5f\x72\x65\x70\x6c\x69\x63\x61\x5f\x5f\x12\x09\x72\x65\x70\x6c\x69\x63\x61\x2d\x31\x12\x14\x09\x00"
            "\x00\x00\x00\x00\x5c\xb1\x40\x10\xe0\xc6\x9c\x8d\xec\xcf\xff\xff\xff\x01\x1a\x15\x12\x05\x74\x65\x73\x74\x31\x22\x0c\x74\x65\x73\x74\x20\x63\x6f"
            "\x75\x6e\x74\x65\x72\x1a\x12\x12\x05\x74\x65\x73\x74\x32\x2a\x09\x74\x65\x73\x74\x20\x75\x6e\x69\x74\x1a\x09\x08\x01\x12\x05\x74\x65\x73\x74\x33"sv,
        .metadata = {Metadata{.metric_name = "test1", .text = "test counter", .type = MetadataType::kHelp},
                     Metadata{.metric_name = "test2", .text = "test unit", .type = MetadataType::kUnit},
                     Metadata{.metric_name = "test3", .text = "COUNTER", .type = MetadataType::kType}},
        .metrics = {Metric{.timeseries = {LabelViewSet{
                                              {"__name__", "test"},
                                              {"__replica__", "replica-1"},
                                              {"cluster", "cluster-0"},
                                          },
                                          BareBones::Vector<Sample>{Sample{-1654608420000, 4444}}}}}}));

}  // namespace