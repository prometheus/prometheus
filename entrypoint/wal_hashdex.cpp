#include <cstdint>

#include "_helpers.hpp"
#include "wal_hashdex.h"

#include "primitives/go_slice.h"
#include "wal/hashdex.h"

extern "C" void prompp_wal_hashdex_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Hashdex::label_set_limits limits;
  };
  struct Result {
    PromPP::WAL::Hashdex* hashdex;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();
  out->hashdex = new PromPP::WAL::Hashdex(in->limits);
}

extern "C" void prompp_wal_hashdex_dtor(void* args) {
  struct Arguments {
    PromPP::WAL::Hashdex* hashdex;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->hashdex;
}

extern "C" void prompp_wal_hashdex_presharding(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Hashdex* hashdex;
    PromPP::Primitives::Go::SliceView<char> protobuf;
  };
  struct Result {
    PromPP::Primitives::Go::String cluster;
    PromPP::Primitives::Go::String replica;
    PromPP::Primitives::Go::Slice<char> error;
  };
  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->hashdex->presharding(in->protobuf.data(), in->protobuf.size());
    auto cluster = in->hashdex->cluster();
    out->cluster.reset_to(cluster.data(), cluster.size());
    auto replica = in->hashdex->replica();
    out->replica.reset_to(replica.data(), replica.size());
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}
