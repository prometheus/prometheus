#include <gtest/gtest.h>

#include "wal/wal_c_encoder_api.h"

#include <iostream>

int some_encoder = okdb_wal_initialize_encoder();
TEST(EncoderSmokeCheckIntCount, EncoderSmokeTest) {
  std::cout << some_encoder;
}

TEST(EncoderSmokeCheckCreateDestroy, EncoderSmokeTest) {
  auto encoder = okdb_wal_c_encoder_ctor(0, 0);
  okdb_wal_c_encoder_dtor(encoder);
}
