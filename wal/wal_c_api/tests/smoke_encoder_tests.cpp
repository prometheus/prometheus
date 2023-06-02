#include <gtest/gtest.h>

#include "wal/wal_c_encoder_api.h"

#include <iostream>

extern "C" int okdb_wal_initialize_encoder();
int some_encoder = okdb_wal_initialize_encoder();
TEST(EncoderSmokeCheckIntCount, EncoderSmokeTest) {
  std::cout << some_encoder;
}
