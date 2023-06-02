#include <gtest/gtest.h>

#include "wal/wal_c_api.h"

TEST(WalCApi, ReaderSmokeTest) {
  okdb_wal_initialize();
  auto decoder = okdb_wal_basic_decoder_create();
  okdb_wal_basic_decoder_destroy(decoder);
}
