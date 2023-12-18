#include <cstdint>

#include "_helpers.hpp"
#include "wal_encoder.h"

#include "primitives/go_slice.h"
#include "wal/encoder.h"
#include "wal/hashdex.h"
#include "wal/wal.h"

extern "C" void prompp_wal_encoder_ctor(void* args, void* res) {
  struct Arguments {
    uint16_t shard_id;
    uint8_t log_shards;
  };
  struct Result {
    PromPP::WAL::Encoder* encoder;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();
  out->encoder = new PromPP::WAL::Encoder(in->shard_id, in->log_shards);
}

extern "C" void prompp_wal_encoder_dtor(void* args) {
  struct Arguments {
    PromPP::WAL::Encoder* encoder;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->encoder;
}

extern "C" void prompp_wal_encoder_add(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Encoder* encoder;
    PromPP::WAL::Hashdex* hashdex;
  };
  struct Result {
    uint32_t samples;
    uint32_t series;
    int64_t earliest_timestamp;
    int64_t latest_timestamp;
    uint32_t remainder_size;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->encoder->add(*in->hashdex, out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_encoder_add_with_stale_nans(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Encoder* encoder;
    PromPP::WAL::Hashdex* hashdex;
    int64_t stale_ts;
    PromPP::WAL::Writer::SourceState source_state;
  };
  struct Result {
    uint32_t samples;
    uint32_t series;
    int64_t earliest_timestamp;
    int64_t latest_timestamp;
    uint32_t remainder_size;
    PromPP::WAL::Writer::SourceState source_state;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    out->source_state = in->encoder->add_with_stalenans(*in->hashdex, out, in->stale_ts, in->source_state);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_encoder_collect_source(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Encoder* encoder;
    int64_t stale_ts;
    PromPP::WAL::Writer::SourceState source_state;
  };
  struct Result {
    uint32_t samples;
    uint32_t series;
    int64_t earliest_timestamp;
    int64_t latest_timestamp;
    uint32_t remainder_size;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->encoder->collect_source(out, in->stale_ts, in->source_state);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_encoder_finalize(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Encoder* encoder;
  };
  struct Result {
    uint32_t samples;
    uint32_t series;
    int64_t earliest_timestamp;
    int64_t latest_timestamp;
    uint32_t remainder_size;
    PromPP::Primitives::Go::Slice<char> segment;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  auto out_stream = PromPP::Primitives::Go::BytesStream(&out->segment);

  try {
    in->encoder->finalize(out, out_stream);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}
