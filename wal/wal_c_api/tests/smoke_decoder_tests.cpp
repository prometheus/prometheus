#include <gtest/gtest.h>

#include "wal/wal_c_decoder_api.h"

#include <iostream>

int some_decoder = okdb_wal_initialize();

TEST(DecoderSmokeCheckIntCount, DecoderSmokeTest) {
  std::cout << some_decoder;
}

TEST(DecoderSmokeCheckCreateDestroy, DecoderSmokeTest) {
  auto decoder = okdb_wal_c_decoder_ctor();
  okdb_wal_c_decoder_dtor(decoder);
}
