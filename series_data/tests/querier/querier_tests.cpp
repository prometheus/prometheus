#include <gtest/gtest.h>

#include "series_data/chunk_finalizer.h"
#include "series_data/encoder.h"
#include "series_data/outdated_sample_encoder.h"
#include "series_data/querier/querier.h"

namespace {

using series_data::ChunkFinalizer;
using series_data::DataStorage;
using series_data::Encoder;
using series_data::OutdatedSampleEncoder;
using series_data::chunk::DataChunk;
using series_data::querier::QueriedChunk;
using series_data::querier::QueriedChunkList;
using series_data::querier::Querier;
using series_data::querier::Query;

struct QuerierCase {
  Query query;
  QueriedChunkList expected;
};

class QuerierFixture : public testing::TestWithParam<QuerierCase> {
 protected:
  DataStorage storage_;
  std::chrono::system_clock clock_;
  OutdatedSampleEncoder<decltype(clock_)> outdated_sample_encoder_{clock_};
  Encoder<decltype(outdated_sample_encoder_)> encoder{storage_, outdated_sample_encoder_};
  Querier querier_{storage_};

  void fill_storage() {
    for (uint32_t ls_id = 0; ls_id < 2; ++ls_id) {
      encoder.encode(ls_id, 1, 1.0);
      encoder.encode(ls_id, 2, 1.0);
      encoder.encode(ls_id, 3, 1.0);
      encoder.encode(ls_id, 4, 1.0);
      encoder.encode(ls_id, 5, 1.0);
      ChunkFinalizer::finalize(storage_, ls_id, storage_.open_chunks[ls_id]);

      encoder.encode(ls_id, 6, 1.0);
      encoder.encode(ls_id, 7, 1.0);
      encoder.encode(ls_id, 8, 1.0);
      encoder.encode(ls_id, 9, 1.0);
      encoder.encode(ls_id, 10, 1.0);
      ChunkFinalizer::finalize(storage_, ls_id, storage_.open_chunks[ls_id]);

      encoder.encode(ls_id, 12, 1.0);
      encoder.encode(ls_id, 13, 1.0);
      encoder.encode(ls_id, 14, 1.0);
    }
  }
};

TEST_F(QuerierFixture, QueryEmptyChunk) {
  // Arrange
  encoder.encode(2, 1, 1.0);

  // Act
  auto& result = querier_.query(Query{.start_timestamp_ms = 1, .end_timestamp_ms = 1, .label_set_ids = {0, 1, 2}});

  // Assert
  EXPECT_EQ(QueriedChunkList{QueriedChunk(2)}, result);
}

TEST_P(QuerierFixture, QueryFilledChunks) {
  // Arrange
  fill_storage();

  // Act
  auto& result = querier_.query(GetParam().query);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(NoChunks,
                         QuerierFixture,
                         testing::Values(QuerierCase{.query = {.start_timestamp_ms = 0, .end_timestamp_ms = 0, .label_set_ids = {0}}, .expected = {}},
                                         QuerierCase{.query = {.start_timestamp_ms = 15, .end_timestamp_ms = 16, .label_set_ids = {0}}, .expected = {}}));

INSTANTIATE_TEST_SUITE_P(
    QueryFinalizedChunk,
    QuerierFixture,
    testing::Values(QuerierCase{.query = {.start_timestamp_ms = 0, .end_timestamp_ms = 1, .label_set_ids = {0}}, .expected = {QueriedChunk(0, 0)}},
                    QuerierCase{.query = {.start_timestamp_ms = 1, .end_timestamp_ms = 1, .label_set_ids = {0}}, .expected = {QueriedChunk(0, 0)}},
                    QuerierCase{.query = {.start_timestamp_ms = 5, .end_timestamp_ms = 5, .label_set_ids = {0}}, .expected = {QueriedChunk(0, 0)}},
                    QuerierCase{.query = {.start_timestamp_ms = 6, .end_timestamp_ms = 6, .label_set_ids = {0}}, .expected = {QueriedChunk(0, 1)}},
                    QuerierCase{.query = {.start_timestamp_ms = 10, .end_timestamp_ms = 11, .label_set_ids = {0}}, .expected = {QueriedChunk(0, 1)}},
                    QuerierCase{.query = {.start_timestamp_ms = 7, .end_timestamp_ms = 10, .label_set_ids = {0}}, .expected = {QueriedChunk(0, 1)}}));

INSTANTIATE_TEST_SUITE_P(
    QueryOpenChunk,
    QuerierFixture,
    testing::Values(QuerierCase{.query = {.start_timestamp_ms = 12, .end_timestamp_ms = 12, .label_set_ids = {0}}, .expected = {QueriedChunk(0)}},
                    QuerierCase{.query = {.start_timestamp_ms = 12, .end_timestamp_ms = 14, .label_set_ids = {0}}, .expected = {QueriedChunk(0)}},
                    QuerierCase{.query = {.start_timestamp_ms = 13, .end_timestamp_ms = 13, .label_set_ids = {0}}, .expected = {QueriedChunk(0)}},
                    QuerierCase{.query = {.start_timestamp_ms = 14, .end_timestamp_ms = 14, .label_set_ids = {0}}, .expected = {QueriedChunk(0)}},
                    QuerierCase{.query = {.start_timestamp_ms = 12, .end_timestamp_ms = 15, .label_set_ids = {0}}, .expected = {QueriedChunk(0)}}));

INSTANTIATE_TEST_SUITE_P(MultipeChunks,
                         QuerierFixture,
                         testing::Values(QuerierCase{.query = {.start_timestamp_ms = 0, .end_timestamp_ms = 7, .label_set_ids = {0}},
                                                     .expected = {QueriedChunk(0, 0), QueriedChunk(0, 1)}}));

INSTANTIATE_TEST_SUITE_P(MultipeChunksMultipleLsIds,
                         QuerierFixture,
                         testing::Values(QuerierCase{.query = {.start_timestamp_ms = 0, .end_timestamp_ms = 7, .label_set_ids = {0, 1}},
                                                     .expected = {QueriedChunk(0, 0), QueriedChunk(0, 1), QueriedChunk(1, 0), QueriedChunk(1, 1)}}));

INSTANTIATE_TEST_SUITE_P(NonExistingChunk,
                         QuerierFixture,
                         testing::Values(QuerierCase{.query = {.start_timestamp_ms = 0, .end_timestamp_ms = 7, .label_set_ids = {2}}, .expected = {}}));

}  // namespace