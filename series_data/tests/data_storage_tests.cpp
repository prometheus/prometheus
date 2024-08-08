#include <gtest/gtest.h>

#include "series_data/data_storage.h"

namespace {

using series_data::DataStorage;
using series_data::chunk::DataChunk;
using EncodingType = series_data::chunk::DataChunk::EncodingType;
using ChunkType = series_data::chunk::DataChunk::Type;

struct DataChunkInfo {
  DataChunk chunk;
  uint32_t series_id;
  ChunkType type;

  bool operator==(const DataChunkInfo&) const noexcept = default;
};

struct DataStorageIteratorCase {
  std::vector<DataChunk> open_chunks{};
  std::vector<std::vector<DataChunk>> finalized_chunks{};
  std::vector<DataChunkInfo> expected_chunks{};
};

constexpr uint32_t kDefaultSeriesId = 0;
constexpr DataChunk kOpenChunk = DataChunk(0, 1, EncodingType::kGorilla);

class DataStorageIteratorTrait : public testing::TestWithParam<DataStorageIteratorCase> {
 protected:
  DataStorage data_storage_;

  void SetUp() override {
    std::ranges::copy(GetParam().open_chunks, std::back_inserter(data_storage_.open_chunks));

    for (uint32_t ls_id = 0; ls_id < GetParam().finalized_chunks.size(); ++ls_id) {
      for (auto& chunk : GetParam().finalized_chunks[ls_id]) {
        data_storage_.finalized_chunks.try_emplace(ls_id, data_storage_.finalized_chunks_map_allocated_memory)
            .first->second.emplace(chunk, [](const DataChunk&) { return 0; });
      }
    }
  }
};

class DataStorageSeriesChunkIterator : public DataStorageIteratorTrait {};

TEST_P(DataStorageSeriesChunkIterator, OpenedChunk) {
  // Arrange
  std::vector<DataChunkInfo> chunks;

  // Act
  std::ranges::transform(data_storage_.chunks(kDefaultSeriesId), std::back_inserter(chunks),
                         [](const auto& data) { return DataChunkInfo{.chunk = data.chunk(), .series_id = data.series_id(), .type = data.chunk_type()}; });

  // Assert
  EXPECT_EQ(GetParam().expected_chunks, chunks);
}

INSTANTIATE_TEST_SUITE_P(NoChunks, DataStorageSeriesChunkIterator, testing::Values(DataStorageIteratorCase{}));

INSTANTIATE_TEST_SUITE_P(OpenedChunk,
                         DataStorageSeriesChunkIterator,
                         testing::Values(DataStorageIteratorCase{
                             .open_chunks = {kOpenChunk},
                             .expected_chunks = {DataChunkInfo{.chunk = kOpenChunk, .series_id = kDefaultSeriesId, .type = ChunkType::kOpen}}}));

INSTANTIATE_TEST_SUITE_P(
    FinalizedChunk,
    DataStorageSeriesChunkIterator,
    testing::Values(
        DataStorageIteratorCase{.open_chunks = {kOpenChunk},
                                .finalized_chunks = {{DataChunk(1, 2, EncodingType::kValuesGorilla)}},
                                .expected_chunks = {DataChunkInfo{.chunk = DataChunk(1, 2, EncodingType::kValuesGorilla),
                                                                  .series_id = kDefaultSeriesId,
                                                                  .type = ChunkType::kFinalized},
                                                    DataChunkInfo{.chunk = kOpenChunk, .series_id = kDefaultSeriesId, .type = ChunkType::kOpen}}},
        DataStorageIteratorCase{
            .open_chunks = {kOpenChunk},
            .finalized_chunks = {{DataChunk(1, 2, EncodingType::kValuesGorilla), DataChunk(2, 3, EncodingType::kUint32Constant)}},
            .expected_chunks = {
                DataChunkInfo{.chunk = DataChunk(1, 2, EncodingType::kValuesGorilla), .series_id = kDefaultSeriesId, .type = ChunkType::kFinalized},
                DataChunkInfo{.chunk = DataChunk(2, 3, EncodingType::kUint32Constant), .series_id = kDefaultSeriesId, .type = ChunkType::kFinalized},
                DataChunkInfo{.chunk = kOpenChunk, .series_id = kDefaultSeriesId, .type = ChunkType::kOpen},
            }}));

class DataStorageChunkIterator : public DataStorageIteratorTrait {};

TEST_P(DataStorageChunkIterator, Test) {
  // Arrange
  std::vector<DataChunkInfo> chunks;

  // Act
  std::ranges::transform(data_storage_.chunks(), std::back_inserter(chunks),
                         [](const auto& data) { return DataChunkInfo{.chunk = data.chunk(), .series_id = data.series_id(), .type = data.chunk_type()}; });

  // Assert
  EXPECT_EQ(GetParam().expected_chunks, chunks);
}

INSTANTIATE_TEST_SUITE_P(NoChunks, DataStorageChunkIterator, testing::Values(DataStorageIteratorCase{}));

INSTANTIATE_TEST_SUITE_P(
    OpenChunks,
    DataStorageChunkIterator,
    testing::Values(
        DataStorageIteratorCase{.open_chunks = {DataChunk{0, 1, EncodingType::kGorilla}},
                                .expected_chunks = {DataChunkInfo{.chunk = DataChunk(0, 1, EncodingType::kGorilla), .series_id = 0, .type = ChunkType::kOpen}}},
        DataStorageIteratorCase{
            .open_chunks = {DataChunk{0, 1, EncodingType::kGorilla}, DataChunk{1, 2, EncodingType::kUint32Constant}},
            .expected_chunks = {DataChunkInfo{.chunk = DataChunk(0, 1, EncodingType::kGorilla), .series_id = 0, .type = ChunkType::kOpen},
                                DataChunkInfo{.chunk = DataChunk(1, 2, EncodingType::kUint32Constant), .series_id = 1, .type = ChunkType::kOpen}}}));

INSTANTIATE_TEST_SUITE_P(FinalizedChunks,
                         DataStorageChunkIterator,
                         testing::Values(DataStorageIteratorCase{
                             .open_chunks = {DataChunk{0, 1, EncodingType::kGorilla}, DataChunk{1, 2, EncodingType::kUint32Constant}},
                             .finalized_chunks = {{}, {DataChunk{2, 3, EncodingType::kAscIntegerValuesGorilla}}},
                             .expected_chunks = {
                                 DataChunkInfo{.chunk = DataChunk(0, 1, EncodingType::kGorilla), .series_id = 0, .type = ChunkType::kOpen},
                                 DataChunkInfo{.chunk = DataChunk(2, 3, EncodingType::kAscIntegerValuesGorilla), .series_id = 1, .type = ChunkType::kFinalized},
                                 DataChunkInfo{.chunk = DataChunk(1, 2, EncodingType::kUint32Constant), .series_id = 1, .type = ChunkType::kOpen}}}));

}  // namespace