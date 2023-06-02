#include "gtest/gtest.h"

#include "bare_bones/encoding.h"
#include "bare_bones/zigzag.h"
#include "prometheus/block.h"
#include "prometheus/blocks_chunks.h"

namespace {  // NOLINT

const size_t NUM_BLOCKS = 10;
const size_t NUM_CHUNKS = 5;
const uint64_t STEP_REF = 200;
const int64_t START_MIN_TS = 1621900800000;
const int64_t START_MAX_TS = 1621904400000;
const int64_t STEP_TS = 3600000;

PromPP::Prometheus::Block::Chunks generate_chunks(uint64_t start_ref, size_t i) {
  PromPP::Prometheus::Block::Chunks chunks;
  size_t n = 0;
  if (i % 2 == 0) {
    n = NUM_CHUNKS;
  } else {
    n = NUM_CHUNKS + 1;
  }

  for (size_t i = 0; i < n; i++) {
    chunks.add(START_MIN_TS + STEP_TS * i, START_MAX_TS + STEP_TS * i, start_ref + STEP_REF * i);
  }

  return chunks;
};

struct TestChunks : public testing::Test {};

TEST_F(TestChunks, BlocksChunks) {
  PromPP::Prometheus::Block::BlocksChunks outcomes(START_MIN_TS);
  BareBones::Vector<PromPP::Prometheus::Block::Chunks> etalons;

  for (size_t i = 0; i < NUM_BLOCKS; i++) {
    etalons.push_back(generate_chunks(i * NUM_CHUNKS * STEP_REF, i));
    outcomes.push_back(generate_chunks(i * NUM_CHUNKS * STEP_REF, i));
  }

  auto outcome = outcomes.begin();
  auto etalon = etalons.begin();
  while (outcome != outcomes.end() && etalon != etalons.end()) {
    auto outcome_chunks = (*outcome).begin();
    auto etalon_chunks = (*etalon).begin();
    while (outcome_chunks != (*outcome).end() && etalon_chunks != (*etalon).end()) {
      EXPECT_EQ(*outcome_chunks++, *etalon_chunks++);
    }
    EXPECT_EQ(outcome_chunks == (*outcome).end(), etalon_chunks == (*etalon).end());
    EXPECT_EQ((*outcome).size(), (*etalon).size());

    ++outcome;
    ++etalon;
  }

  EXPECT_EQ(outcome == outcomes.end(), etalon == etalons.end());
}
}  // namespace
