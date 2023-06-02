#include <gtest/gtest.h>

#include "wal/wal_c_types_api.h"

#include <iostream>

extern "C" int okdb_wal_initialize_types();
int some_types = okdb_wal_initialize_types();
TEST(TypesSmokeCheckIntCount, TypesSmokeTest) {
  std::cout << some_types;
}
