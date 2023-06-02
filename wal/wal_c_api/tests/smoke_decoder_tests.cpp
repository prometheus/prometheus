#include <gtest/gtest.h>

#include "wal/wal_c_decoder_api.h"

#include <iostream>

extern "C" int okdb_wal_initialize_decoder();
int some_decoder = okdb_wal_initialize_decoder();
TEST(DecoderSmokeCheckIntCount, DecoderSmokeTest) {
  std::cout << some_decoder;
}
