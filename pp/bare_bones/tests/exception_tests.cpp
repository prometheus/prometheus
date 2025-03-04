#include "bare_bones/exception.h"

#include <filesystem>

#include <gtest/gtest.h>

#include <sys/types.h>  // pid_t
#include <sys/wait.h>   // waitpid()
#include <unistd.h>     // fork()

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

template <typename Callable>
struct CFunctor {
  Callable& c;
  static void call(void* ptr, pid_t pid) { static_cast<CFunctor<Callable>*>(ptr)->c(pid); }
  explicit CFunctor(Callable& c) : c(c) {}
};
template <typename Callable>
CFunctor(Callable c) -> CFunctor<Callable>;

// This test is disabled because it requires the specific filename for checking
// (you can set it via PP_BARE_BONES_EXCEPTION_TEST_COREDUMP_NAME="$(sudo sysctl kernel.core_pattern)")
// This test also DON'T PARSE THE %p, %e, etc. COREDUMP SPECIFIERS (see man core(5) for details),
// IT TRIES TO OPEN THE FILE FROM ENV (with appended ".<pid>" from parent process) DIRECTLY.
// This test mostly depends on exception logic, which is intended to not changing with times.
// You may run it via ./<test_binary> --gtest_filter=*CoreDumpTest --gtest_also_run_disabled_tests
// This test also depends on PP_BARE_BONES_EXCEPTION_TEST_SHOULD_FAIL environment variable.
// In that case the tested exception won't throw and the coredump wouldn't generated, which leads to fail.
// See https://www.baeldung.com/linux/managing-core-dumps for coredumps tutorial.
// The resulting coredump may be analized via gdb --core=<coredump>
TEST(ExceptionDeathTest, DISABLED_CoreDumpTest) {
  auto filename = getenv("PP_BARE_BONES_EXCEPTION_TEST_COREDUMP_NAME");
  if (filename == nullptr) {
    GTEST_SKIP() << "This test requires PP_BARE_BONES_EXCEPTION_TEST_COREDUMP_NAME environment variable to set "
                    "for proper coredump filename. You may get the current coredump file names pattern via "
                    "`sysctl kernel.core_pattern` (possibly running as sudo/root user), but this test doesn't "
                    "support neither of file patterns (except of exact filename).\n";
    FAIL();  // this test is not intended to run w/o environment
  }
  std::filesystem::path coredump_file_path(filename);
  if (!coredump_file_path.is_absolute()) {
    GTEST_SKIP() << "This test requires absolute path in PP_BARE_BONES_EXCEPTION_TEST_COREDUMP_NAME "
                    "environment variable for proper coredump filename. You may get the current coredump file names pattern via "
                    "`sysctl kernel.core_pattern` (possibly running as sudo/root user), but this test doesn't "
                    "support neither of file patterns (except of exact filename).\n";
    FAIL();  // this test is not intended to run w/o environment
  } else {
  }

  auto fork_handler = [&](pid_t pid) {
    if (pid > 0) {
      std::cout << "Waiting for child " << pid << "...\n";
      std::string s = filename;
      s += ".";
      s += std::to_string(pid);
      coredump_file_path = s;
      std::cout << "Will check that file " << coredump_file_path << " after child PID " << pid << " exits\n";
      waitpid(pid, nullptr, 0);
    }
  };
  CFunctor c_handler(fork_handler);
  prompp_enable_coredumps_on_exception(1);
  prompp_barebones_exception_set_on_fork_handler(&c_handler, decltype(c_handler)::call);

  try {
    if (getenv("PP_BARE_BONES_EXCEPTION_TEST_SHOULD_FAIL") == nullptr) {
      throw BareBones::Exception(0x3941393742d8cc07, "This exception would terminate fork()-ed child process");
    }
  } catch (...) {
    // mask exception, as intended.
  }

  EXPECT_TRUE(std::filesystem::exists(coredump_file_path));

  prompp_enable_coredumps_on_exception(0);
}

}  // namespace
