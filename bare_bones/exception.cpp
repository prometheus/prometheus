#include "bare_bones/exception.h"

#include <exception>
#include <limits>

#include <errno.h>     // errno
#include <inttypes.h>  // printf() specifiers for long types.
#include <stdarg.h>    // va_arg
#include <stdlib.h>    // getenv()
#include <string.h>    // strerror()

// linux-specific includes for coredumps
#include <strings.h>       // strncasecmp()
#include <sys/resource.h>  // setrlimit() for coredumps
#include <sys/types.h>     // pid_t
#include <unistd.h>        // fork()

static bool coredump_enabled = false;
static void (*onfork_handler)(void*, pid_t) = [](void*, pid_t) {};
static void* onfork_handler_state = nullptr;

extern "C" void prompp_enable_coredumps_on_exception(int enable) {
  coredump_enabled = enable != 0;
}

// this handler would be called only in parent process.
extern "C" void prompp_barebones_exception_set_on_fork_handler(void* state, void (*handler)(void* state, pid_t pid)) {
  onfork_handler = handler;
  onfork_handler_state = state;
}

namespace BareBones {
static constexpr size_t user_buffer_msg_size = 255;

static constexpr size_t get_exception_buffer_size() {
  // the HEX representation for type is twice longer than their sizeof() as the 1 byte is written as two HEX digits.
  constexpr size_t byte_size_in_printed_characters = 2;
  // total buffer size must be sufficient for storing the "Exception <HEX-code>: " + user defined message. +1 for \0 (it's got from std::size())
  return user_buffer_msg_size + sizeof(Exception::Code) * byte_size_in_printed_characters + std::size("Exception : ");
}

Exception::Exception(Code exc_code, const char* message, ...) : code_(exc_code) {
  if (coredump_enabled) {
    auto pid = fork();
    // > 0 is the parent, == 0 is the child.
    if (pid > 0) {
      // parent process, empty block.
      onfork_handler(onfork_handler_state, pid);  // fork succeeded, call handler in parent process
    } else if (pid < 0) {
      onfork_handler(onfork_handler_state, pid);  // fork failed
      int err = errno;
      fprintf(stderr, "%s: can't fork() for getting coredump (Error %d: %s)\n", __func__, err, strerror(err));
    } else /* if (pid == 0) */ {
      // child process, don't need to call fork() handler - it's the main logic.
      // expose max limits for coredump size
      rlimit limits{
          .rlim_cur = RLIM_INFINITY,
          .rlim_max = RLIM_INFINITY,
      };
      auto val = setrlimit(RLIMIT_CORE, &limits);
      if (val < 0) {
        // oops, error
        auto err = errno;
        fprintf(stderr, "%s (child process): warning: can't change rlimit for coredump files, coredump file may not be created (Error %d: %s)\n", __func__, err,
                strerror(err));
      }
      std::terminate();
    }
  }
  constexpr auto sz = get_exception_buffer_size();
  thread_local char buf[sz];

  buf[sz - 1] = '\0';
  size_t off1 = snprintf(buf, sz, "Exception %" PRIx64 ": ", exc_code);
  va_list list;
  va_start(list, message);

  // Concatenate the user-formatted message (+1 for \0 as snprintf() truncates if formatted string fills the whole buffer)
  size_t l = vsnprintf(buf + off1, user_buffer_msg_size + 1, message, list);
  msg_ = std::string_view(buf, off1 + l + 1);  // +1 for \0
}

}  // namespace BareBones
