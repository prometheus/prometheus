#include "bare_bones/exception.h"
#include "bare_bones/vector.h"

int main() {
  prompp_enable_coredumps_on_exception(1);
  BareBones::Vector<int> vec;
  return vec[4];
}
