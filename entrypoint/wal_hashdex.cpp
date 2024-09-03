#include <variant>

#include "_helpers.hpp"
#include "wal_hashdex.h"

#include "primitives/go_slice.h"
#include "wal/decoder.h"
#include "wal/hashdex.h"

extern "C" void prompp_wal_protobuf_hashdex_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::HashdexLimits limits;
  };
  struct Result {
    HashdexVariant* hashdex;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();
  out->hashdex = new HashdexVariant{std::in_place_index<HashdexType::protobuf>, in->limits};
}

void prompp_wal_hashdex_dtor(void* args) {
  struct Arguments {
    HashdexVariant* hashdex;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->hashdex;
}

extern "C" void prompp_wal_protobuf_hashdex_dtor(void* args) {
  prompp_wal_hashdex_dtor(args);
}

extern "C" void prompp_wal_protobuf_hashdex_presharding(void* args, void* res) {
  struct Arguments {
    HashdexVariant* hashdex_variant;
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
    auto& hashdex = std::get<PromPP::WAL::ProtobufHashdex>(*in->hashdex_variant);
    hashdex.presharding(in->protobuf.data(), in->protobuf.size());
    auto cluster = hashdex.cluster();
    out->cluster.reset_to(cluster.data(), cluster.size());
    auto replica = hashdex.replica();
    out->replica.reset_to(replica.data(), replica.size());
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_go_model_hashdex_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::HashdexLimits limits;
  };
  struct Result {
    HashdexVariant* hashdex;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();
  out->hashdex = new HashdexVariant{std::in_place_index<HashdexType::go_model>, in->limits};
}

extern "C" void prompp_wal_go_model_hashdex_dtor(void* args) {
  prompp_wal_hashdex_dtor(args);
}

extern "C" void prompp_wal_go_model_hashdex_presharding(void* args, void* res) {
  struct Arguments {
    HashdexVariant* hashdex_variant;
    PromPP::Primitives::Go::SliceView<PromPP::Primitives::Go::TimeSeries> data;
  };
  struct Result {
    PromPP::Primitives::Go::String cluster;
    PromPP::Primitives::Go::String replica;
    PromPP::Primitives::Go::Slice<char> error;
  };
  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    auto& hashdex = std::get<PromPP::WAL::GoModelHashdex>(*in->hashdex_variant);
    hashdex.presharding(in->data);
    auto cluster = hashdex.cluster();
    out->cluster.reset_to(cluster.data(), cluster.size());
    auto replica = hashdex.replica();
    out->replica.reset_to(replica.data(), replica.size());
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_basic_decoder_hashdex_dtor(void* args) {
  prompp_wal_hashdex_dtor(args);
}
