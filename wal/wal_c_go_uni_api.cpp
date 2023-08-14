/// \file wal/wal_c_go_bridged_api.cpp
/// Contains the unified C <-> Go API.
// The so-called "unified" (uni) API is a convention for exported C functions'
// arguments. The all actual arguments are folded in a C struct and this struct
// is forwarded to unified function. The possible function prototypes may look
// like:
// - factory_fn(in_args, out_args, error_info);
// - api_fn_without_return_values(object_handle, in_args, error_info);
// - api_fn_with_return_values(object_handle, in_args, out_args, error_info);

#include "wal_c_decoder_api.h"
#include "wal_c_encoder_api.h"
#include "wal_c_types_api.h"

#include <exception>
#include <sstream>
#include <string>

#include "bare_bones/exception.h"

#if __has_include("bare_bones/stacktrace.h")
#include "bare_bones/stacktrace.h"
#define CURRENT_STACKTRACE BareBones::StackTrace::Current().ToString()
#elif __has_include(<stacktrace>)  // sanity checks..
#if __cplusplus <= 202002L
#error "Please set -std="c++2b" or similar flag for C++23 for your compiler."
#endif
#include <stacktrace>
#if defined(_GLIBCXX_HAVE_STACKTRACE) && (_GLIBCXX_HAVE_STACKTRACE != 0)
#define HAVE_STACKTRACE 1
#define CURRENT_STACKTRACE std::to_string(std::stacktrace::current())
#else
#define HAVE_STACKTRACE 0
#define CURRENT_STACKTRACE ""
#endif

#else
#error "Your C++ Standard library doesn't implement the std::stacktrace. Make sure that you use conformant Library (e.g., libstdc++ from GCC 12)"
#endif

// helper for __func__ and pretty message in c_api_error_info.
static c_api_error_info* make_c_api_error_info_with_func_name(std::string_view func_name, std::string&& message, std::string&& stacktrace) {
  std::stringstream ss;
  ss << func_name << "(): " << message;
  return new c_api_error_info(ss.str(), std::move(stacktrace));
}

#define MAKE_C_API_ERROR(message) make_c_api_error_info_with_func_name(__func__, (message), CURRENT_STACKTRACE)

static c_api_error_info* handle_current_exception(std::string_view func_name, std::string&& stacktrace, std::exception_ptr ep) {
  std::stringstream ss;
  try {
    std::rethrow_exception(ep);
  } catch (const BareBones::Exception& e) {
    return make_c_api_error_info_with_func_name(func_name, e.what(), std::move(stacktrace));
  } catch (const std::exception& e) {
    ss << "caught a std::exception, what: " << e.what();
  } catch (...) {
    ss << "caught an unknown exception";
  }
  return make_c_api_error_info_with_func_name(func_name, ss.str(), std::move(stacktrace));
}

// c_api_error_info public API.

extern "C" c_api_error_info* make_c_api_error_info(const char* message, const char* stacktrace) {
  return new c_api_error_info(std::string(message ? message : ""), std::string(stacktrace ? stacktrace : ""));
}

extern "C" void destroy_c_api_error_info(c_api_error_info* err_info) {
  delete err_info;
}

extern "C" const char* c_api_error_info_get_error(c_api_error_info* err) {
  return err->Error().data();
}

extern "C" const char* c_api_error_info_get_stacktrace(c_api_error_info* err) {
  return err->Stacktrace().data();
}

// BasicEncoder unified API

extern "C" void okdb_wal_uni_c_encoder_ctor(c_encoder_ctor_params* in_ctor_args, c_encoder* out_encoder_ptr, c_api_error_info** err) {
  if (!in_ctor_args) {
    *err = MAKE_C_API_ERROR("null pointer to in_ctor_args");
    return;
  }
  if (!out_encoder_ptr) {
    *err = MAKE_C_API_ERROR("null pointer to return value (out_encoder_ptr)");
    return;
  }
  try {
    *out_encoder_ptr = okdb_wal_c_encoder_ctor(in_ctor_args->shard_id, in_ctor_args->number_of_shards);
  } catch (...) {
    *err = handle_current_exception(__func__, CURRENT_STACKTRACE, std::current_exception());
  }
}

extern "C" void okdb_wal_uni_c_encoder_encode(c_encoder c_enc, c_encoder_encode_params* encode_params, c_api_error_info** err) {
  try {
    okdb_wal_c_encoder_encode(c_enc, encode_params->hashdex, encode_params->segment, encode_params->redundant);
  } catch (...) {
    *err = handle_current_exception(__func__, CURRENT_STACKTRACE, std::current_exception());
  }
}

extern "C" void okdb_wal_uni_c_encoder_add(c_encoder c_enc, c_encoder_add_params* add_params, c_api_error_info** err) {
  try {
    okdb_wal_c_encoder_add(c_enc, add_params->hashdex, add_params->segment);
  } catch (...) {
    *err = handle_current_exception(__func__, CURRENT_STACKTRACE, std::current_exception());
  }
}

extern "C" void okdb_wal_uni_c_encoder_finalize(c_encoder c_enc, c_encoder_finalize_params* finalize_params, c_api_error_info** err) {
  try {
    okdb_wal_c_encoder_finalize(c_enc, finalize_params->segment, finalize_params->redundant);
  } catch (...) {
    *err = handle_current_exception(__func__, CURRENT_STACKTRACE, std::current_exception());
  }
}

extern "C" void okdb_wal_uni_c_encoder_snapshot(c_encoder c_enc, c_encoder_snapshot_params* snapshot_params, c_api_error_info** err) {
  try {
    okdb_wal_c_encoder_snapshot(c_enc, snapshot_params->c_rts, snapshot_params->snapshot);
  } catch (...) {
    *err = handle_current_exception(__func__, CURRENT_STACKTRACE, std::current_exception());
  }
}

// BasicDecoder unified API.
extern "C" void okdb_wal_uni_c_decoder_ctor(c_encoder* out_decoder_ptr, c_api_error_info** err) {
  if (!out_decoder_ptr) {
    *err = MAKE_C_API_ERROR("null pointer to return value (out_decoder_ptr)");
    return;
  }
  try {
    *out_decoder_ptr = okdb_wal_c_decoder_ctor();
  } catch (...) {
    *err = handle_current_exception(__func__, CURRENT_STACKTRACE, std::current_exception());
  }
}

extern "C" void okdb_wal_uni_c_decoder_decode(c_decoder c_dec, c_decoder_decode_params* params, c_decoder_decode_result* out_result, c_api_error_info** err) {
  try {
    out_result->result = okdb_wal_c_decoder_decode(c_dec, params->segment, params->protobuf);
  } catch (...) {
    *err = handle_current_exception(__func__, CURRENT_STACKTRACE, std::current_exception());
  }
}

extern "C" void okdb_wal_uni_c_decoder_decode_dry(c_decoder c_dec,
                                                  c_decoder_decode_dry_params* params,
                                                  c_decoder_decode_result* out_result,
                                                  c_api_error_info** err) {
  try {
    out_result->result = okdb_wal_c_decoder_decode_dry(c_dec, params->segment);
  } catch (...) {
    *err = handle_current_exception(__func__, CURRENT_STACKTRACE, std::current_exception());
  }
}

extern "C" void okdb_wal_uni_c_decoder_snapshot(c_decoder c_dec, c_decoder_snapshot_params* params, c_api_error_info** err) {
  try {
    okdb_wal_c_decoder_snapshot(c_dec, params->snapshot);
  } catch (...) {
    *err = handle_current_exception(__func__, CURRENT_STACKTRACE, std::current_exception());
  }
}

// CHashdex API.

extern "C" void okdb_wal_uni_c_hashdex_ctor(c_hashdex* out_hashdex_ptr, c_api_error_info** err) {
  try {
    *out_hashdex_ptr = okdb_wal_c_hashdex_ctor();
  } catch (...) {
    *err = handle_current_exception(__func__, CURRENT_STACKTRACE, std::current_exception());
  }
}

extern "C" void okdb_wal_uni_c_hashdex_presharding(c_hashdex c_hx, c_hashdex_presharding_params* params, c_api_error_info** err) {
  try {
    okdb_wal_c_hashdex_presharding(c_hx, params->proto_data, params->cluster, params->replica);
  } catch (...) {
    *err = handle_current_exception(__func__, CURRENT_STACKTRACE, std::current_exception());
  }
}
