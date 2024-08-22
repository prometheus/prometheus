#include <spanstream>

#include <gmock/gmock.h>

#include "primitives/lss_metadata/changes_collector.h"
#include "primitives/lss_metadata/storage.h"

namespace {

using PromPP::Primitives::lss_metadata::ChangesCollector;
using PromPP::Primitives::lss_metadata::ImmutableStorage;
using PromPP::Primitives::lss_metadata::MutableStorage;
using PromPP::Primitives::lss_metadata::NopChangesCollector;
using Metadata = PromPP::Primitives::lss_metadata::Metadata;
using std::operator""sv;

class LssMetadataStorageFixture : public testing::Test {
 protected:
  MutableStorage<ChangesCollector> lss_metadata_;
};

TEST_F(LssMetadataStorageFixture, Add) {
  // Arrange

  // Act
  lss_metadata_.add(0, {.help = "help", .type = "type", .unit = "unit"});

  // Assert
  EXPECT_EQ((Metadata{.help = "help", .type = "type", .unit = "unit"}), lss_metadata_.get(0));
}

TEST_F(LssMetadataStorageFixture, UseLatestHelp) {
  // Arrange
  lss_metadata_.add(0, {.help = "help", .type = "type", .unit = "unit"});

  // Act
  lss_metadata_.add(0, {.help = "help1", .type = "new_type", .unit = "unit"});
  lss_metadata_.add(0, {.help = "help", .type = "new_type", .unit = "unit"});

  // Assert
  EXPECT_EQ((Metadata{.help = "help1", .type = "type", .unit = "unit"}), lss_metadata_.get(0));
}

TEST_F(LssMetadataStorageFixture, TypeIsImmutable) {
  // Arrange
  lss_metadata_.add(0, {.help = "help", .type = "type", .unit = "unit"});

  // Act
  lss_metadata_.add(0, {.help = "help", .type = "new_type", .unit = "unit"});

  // Assert
  EXPECT_EQ((Metadata{.help = "help", .type = "type", .unit = "unit"}), lss_metadata_.get(0));
}

TEST_F(LssMetadataStorageFixture, UnitIsImmutable) {
  // Arrange
  lss_metadata_.add(0, {.help = "help", .type = "type", .unit = "unit"});

  // Act
  lss_metadata_.add(0, {.help = "help", .type = "type", .unit = "new_unit"});

  // Assert
  EXPECT_EQ((Metadata{.help = "help", .type = "type", .unit = "unit"}), lss_metadata_.get(0));
}

TEST_F(LssMetadataStorageFixture, GetUnexistingMetadata) {
  // Arrange

  // Act
  auto metadata = lss_metadata_.get(0);

  // Assert
  EXPECT_EQ((Metadata{}), metadata);
}

class LssMetadataStorageDeltaFixture : public testing::Test {
 protected:
  MutableStorage<ChangesCollector> lss_metadata_;
  std::stringstream stream_;
};

TEST_F(LssMetadataStorageDeltaFixture, EmptyDelta) {
  // Arrange

  // Act
  lss_metadata_.write_changes(stream_);

  // Assert
  EXPECT_EQ("\x00"sv, stream_.view());
}

TEST_F(LssMetadataStorageDeltaFixture, FirstDelta) {
  // Arrange
  lss_metadata_.add(0, {.help = "help", .type = "type", .unit = "unit"});

  // Act
  lss_metadata_.write_changes(stream_);

  // Assert
  EXPECT_EQ(
      "\x01"

      "\x01"
      "\x01"
      "\x03\x00\x00\x00"

      "\x00\x00\x00\x00"
      "\x04\x00\x00\x00"
      "\x04\x00\x00\x00"
      "\x04\x00\x00\x00"
      "\x08\x00\x00\x00"
      "\x04\x00\x00\x00"

      "\x01"
      "\x01"
      "\x0C\x00\x00\x00"
      "help"
      "type"
      "unit"

      "\x01\x00\x00\x00"
      "\x00\x00\x00\x00"
      "\x00\x00\x00\x00"
      "\x01\x00\x00\x00"
      "\x02\x00\x00\x00"sv,
      stream_.view());
}

TEST_F(LssMetadataStorageDeltaFixture, AddSymbolInDelta) {
  // Arrange
  lss_metadata_.add(0, {.help = "help", .type = "type", .unit = "unit"});
  lss_metadata_.reset_changes();
  lss_metadata_.add(0, {.help = "new_help", .type = "type", .unit = "unit"});

  // Act
  lss_metadata_.write_changes(stream_);

  // Assert
  EXPECT_EQ(
      "\x01"

      "\x01"
      "\x02"
      "\x03\x00\x00\x00"

      "\x01\x00\x00\x00"
      "\x0C\x00\x00\x00"
      "\x08\x00\x00\x00"

      "\x01"
      "\x02"
      "\x0C\x00\x00\x00"
      "\x08\x00\x00\x00"
      "new_help"

      "\x01\x00\x00\x00"
      "\x00\x00\x00\x00"
      "\x03\x00\x00\x00"
      "\x01\x00\x00\x00"
      "\x02\x00\x00\x00"sv,
      stream_.view());
}

TEST_F(LssMetadataStorageDeltaFixture, UpdateMetadataInDelta) {
  // Arrange
  lss_metadata_.add(0, {.help = "help", .type = "type", .unit = "unit"});
  lss_metadata_.reset_changes();
  lss_metadata_.add(0, {.help = "type", .type = "type", .unit = "unit"});

  // Act
  lss_metadata_.write_changes(stream_);

  // Assert
  EXPECT_EQ(
      "\x01"

      "\x01"
      "\x02"
      "\x03\x00\x00\x00"

      "\x00\x00\x00\x00"

      "\x01\x00\x00\x00"
      "\x00\x00\x00\x00"
      "\x01\x00\x00\x00"
      "\x01\x00\x00\x00"
      "\x02\x00\x00\x00"sv,
      stream_.view());
}

TEST_F(LssMetadataStorageDeltaFixture, AddSymbolAndUpdateMetadataInDelta) {
  // Arrange
  lss_metadata_.add(0, {.help = "help", .type = "type", .unit = "unit"});
  lss_metadata_.reset_changes();
  lss_metadata_.add(0, {.help = "new_help", .type = "type", .unit = "unit"});
  lss_metadata_.add(1, {.help = "new_help", .type = "type", .unit = "unit"});

  // Act
  lss_metadata_.write_changes(stream_);

  // Assert
  EXPECT_EQ(
      "\x01"

      "\x01"
      "\x02"
      "\x03\x00\x00\x00"

      "\x01\x00\x00\x00"
      "\x0C\x00\x00\x00"
      "\x08\x00\x00\x00"

      "\x01"
      "\x02"
      "\x0C\x00\x00\x00"
      "\x08\x00\x00\x00"
      "new_help"

      "\x02\x00\x00\x00"
      "\x00\x00\x00\x00"
      "\x03\x00\x00\x00"
      "\x01\x00\x00\x00"
      "\x02\x00\x00\x00"
      "\x01\x00\x00\x00"
      "\x03\x00\x00\x00"
      "\x01\x00\x00\x00"
      "\x02\x00\x00\x00"sv,
      stream_.view());
}

class LssMetadataStorageDeltaLoaderFixture : public testing::Test {
 protected:
  MutableStorage<ChangesCollector> lss_metadata_;
};

TEST_F(LssMetadataStorageDeltaLoaderFixture, EmptyBuffer) {
  // Arrange
  std::istringstream stream{""};
  MutableStorage<ChangesCollector>::DeltaLoader delta_loader{lss_metadata_};

  // Act

  // Assert
  EXPECT_THAT([&]() { delta_loader.load(stream); },
              testing::Throws<BareBones::Exception>(testing::Property(&BareBones::Exception::code, testing::Eq(0xe46562d01e29e691))));
}

TEST_F(LssMetadataStorageDeltaLoaderFixture, InvalidVersion) {
  // Arrange
  std::ispanstream stream{"\x02"};
  MutableStorage<ChangesCollector>::DeltaLoader delta_loader{lss_metadata_};

  // Act

  // Assert
  EXPECT_THAT([&]() { delta_loader.load(stream); },
              testing::Throws<BareBones::Exception>(testing::Property(&BareBones::Exception::code, testing::Eq(0xe46562d01e29e691))));
}

TEST_F(LssMetadataStorageDeltaLoaderFixture, InvalidBuffer) {
  // Arrange
  std::ispanstream stream{
      "\x01"

      "\x01"
      "\x01"
      "\x00\x00\x00\x00"

      "\x00"sv};
  MutableStorage<ChangesCollector>::DeltaLoader delta_loader{lss_metadata_};

  // Act

  // Assert
  EXPECT_THROW(delta_loader.load(stream), std::runtime_error);
}

class LssMetadataStorageWriteLoadDeltaFixture : public testing::Test {
 protected:
  MutableStorage<ChangesCollector> source_lss_metadata_;
  MutableStorage<NopChangesCollector> destination_lss_metadata_;
  std::stringstream stream_;
};

TEST_F(LssMetadataStorageWriteLoadDeltaFixture, FirstDelta) {
  // Arrange
  source_lss_metadata_.add(10, {.help = "help", .type = "type", .unit = "unit"});
  source_lss_metadata_.add(1, {.help = "help1", .type = "type1", .unit = "unit1"});

  // Act
  source_lss_metadata_.write_changes(stream_);
  stream_ >> destination_lss_metadata_;

  // Assert
  EXPECT_EQ((Metadata{.help = "help", .type = "type", .unit = "unit"}), destination_lss_metadata_.get(10));
  EXPECT_EQ((Metadata{.help = "help1", .type = "type1", .unit = "unit1"}), destination_lss_metadata_.get(1));
}

TEST_F(LssMetadataStorageWriteLoadDeltaFixture, EmptyDelta) {
  // Arrange

  // Act
  source_lss_metadata_.write_changes(stream_);
  stream_ >> destination_lss_metadata_;

  // Assert
}

TEST_F(LssMetadataStorageWriteLoadDeltaFixture, AddMetadataInDelta) {
  // Arrange
  source_lss_metadata_.add(0, {.help = "help", .type = "type", .unit = "unit"});
  destination_lss_metadata_.add(0, {.help = "help", .type = "type", .unit = "unit"});
  source_lss_metadata_.reset_changes();
  source_lss_metadata_.add(1, {.help = "help", .type = "type", .unit = "unit"});

  // Act
  source_lss_metadata_.write_changes(stream_);
  stream_ >> destination_lss_metadata_;

  // Assert
  EXPECT_EQ((Metadata{.help = "help", .type = "type", .unit = "unit"}), destination_lss_metadata_.get(1));
}

TEST_F(LssMetadataStorageWriteLoadDeltaFixture, UpdateMetadaInDelta) {
  // Arrange
  source_lss_metadata_.add(0, {.help = "help", .type = "type", .unit = "unit"});
  destination_lss_metadata_.add(0, {.help = "help", .type = "type", .unit = "unit"});
  source_lss_metadata_.reset_changes();
  source_lss_metadata_.add(0, {.help = "new_help", .type = "type", .unit = "unit"});

  // Act
  source_lss_metadata_.write_changes(stream_);
  stream_ >> destination_lss_metadata_;

  // Assert
  EXPECT_EQ((Metadata{.help = "new_help", .type = "type", .unit = "unit"}), destination_lss_metadata_.get(0));
}

}  // namespace