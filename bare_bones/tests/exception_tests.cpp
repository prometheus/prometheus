#include "bare_bones/exception.h"

#include <gtest/gtest.h>

#define DUPE_5X_LITERAL(literal) literal literal literal literal literal
#define DUPE_25X_LITERAL(literal) \
  DUPE_5X_LITERAL(literal)        \
  DUPE_5X_LITERAL(literal) DUPE_5X_LITERAL(literal) DUPE_5X_LITERAL(literal) DUPE_5X_LITERAL(literal)

namespace {

TEST(ExceptionTest, TruncateTest) {
  BareBones::Exception e(0xcc63de60c4c06e86, "long:%s", DUPE_25X_LITERAL("'123456789"));  // 255 chars
  std::string_view s{e.what()};
  std::cout << "Exception length: " << s.size() << "\n";
  std::cout << "TEST EXCEPTION: " << s;
  EXPECT_GE(s.size(), 255);
  EXPECT_EQ(s[s.size() - 1], '9');
}

}  // namespace
