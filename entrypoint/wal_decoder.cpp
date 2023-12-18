#include <cstdint>

#include "_helpers.hpp"
#include "wal_decoder.h"

#include "primitives/go_slice.h"
#include "primitives/go_slice_protozero.h"
#include "wal/decoder.h"

extern "C" void prompp_wal_decoder_ctor(void* res) {
  using Result = struct {
    PromPP::WAL::Decoder* decoder;
  };

  Result* out = new (res) Result();
  out->decoder = new PromPP::WAL::Decoder();
}

extern "C" void prompp_wal_decoder_dtor(void* args) {
  struct Arguments {
    PromPP::WAL::Decoder* decoder;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->decoder;
}

extern "C" void prompp_wal_decoder_decode(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Decoder* decoder;
    PromPP::Primitives::Go::SliceView<char> segment;
  };
  using Result = struct {
    int64_t created_at;
    int64_t encoded_at;
    uint32_t samples;
    uint32_t series;
    uint32_t segment_id;
    PromPP::Primitives::Go::Slice<char> protobuf;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->decoder->decode(in->segment, out->protobuf, out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_decoder_decode_dry(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Decoder* decoder;
    PromPP::Primitives::Go::SliceView<char> segment;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->decoder->decode_dry(in->segment);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_decoder_restore_from_stream(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Decoder* decoder;
    PromPP::Primitives::Go::SliceView<char> stream;
    uint32_t segment_id;
  };
  struct Result {
    size_t offset = 0;
    uint32_t segment_id;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->decoder->restore_from_stream(in->stream, in->segment_id, out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}
