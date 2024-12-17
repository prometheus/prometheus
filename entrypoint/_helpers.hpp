#pragma once

#include <exception>
#include <string>
#include <string_view>
#include <variant>

#include "bare_bones/exception.h"
#include "wal/hashdex.h"
#include "wal/hashdex/protobuf.h"
#include "wal/hashdex/scraper/scraper.h"

template <class Out>
inline __attribute__((always_inline)) void handle_current_exception(std::string_view func_name, Out& out) {
  std::string msg, st;
  try {
    std::rethrow_exception(std::current_exception());
  } catch (const BareBones::Exception& e) {
    msg = e.what();
    st = e.stacktrace().ToString();
  } catch (const std::exception& e) {
    msg = "caught a std::exception, what: ";
    msg += e.what();
  } catch (...) {
    msg = "caught an unknown exception";
  }
  out.write(func_name.data(), func_name.size());
  out.write("(): ", 4);
  out.write(msg.data(), msg.size());
  out.put('\n');
  out.write(st.data(), st.size());
}

/**
 * used for indexing HashdexVariant.
 */
enum HashdexType : uint8_t {
  kProtobuf = 0,
  kGoModel,
  kDecoder,
  kPrometheusScraper,
  kOpenMetricsScraper,
};
using HashdexVariant = std::variant<PromPP::WAL::hashdex::Protobuf,
                                    PromPP::WAL::GoModelHashdex,
                                    PromPP::WAL::BasicDecoderHashdex,
                                    PromPP::WAL::hashdex::scraper::PrometheusScraper,
                                    PromPP::WAL::hashdex::scraper::OpenMetricsScraper>;
