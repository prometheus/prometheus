#include <gtest/gtest.h>

#include "wal/wal_c_api.h"

#include <iostream>

extern "C" int okdb_wal_initialize();

int some = okdb_wal_initialize();

TEST(DeliverySmokeCheckIntCount, DeliverySmokeTest) {
  std::cout << some;
}
