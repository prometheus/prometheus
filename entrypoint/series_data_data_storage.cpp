#include "series_data_data_storage.h"

#include "chunk_recoder.hpp"
#include "primitives/go_slice.h"
#include "series_data/data_storage.h"

namespace {

using DataStoragePtr = std::unique_ptr<series_data::DataStorage>;

static_assert(sizeof(DataStoragePtr) == sizeof(void*));

};  // namespace

extern "C" void prompp_series_data_data_storage_ctor(void* res) {
  using Result = struct {
    DataStoragePtr data_storage;
  };

  new (res) Result{.data_storage = std::make_unique<series_data::DataStorage>()};
}

extern "C" void prompp_series_data_data_storage_reset(void* args) {
  struct Arguments {
    DataStoragePtr data_storage;
  };

  reinterpret_cast<Arguments*>(args)->data_storage->reset();
}

extern "C" void prompp_series_data_data_storage_dtor(void* args) {
  struct Arguments {
    DataStoragePtr data_storage;
  };

  reinterpret_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_series_data_chunk_recoder_ctor(void* args, void* res) {
  struct Arguments {
    series_data::DataStorage* data_storage;
  };
  struct Result {
    entrypoint::ChunkRecoderPtr chunk_recoder;
  };

  new (res) Result{.chunk_recoder = std::make_unique<entrypoint::ChunkRecoder>(reinterpret_cast<Arguments*>(args)->data_storage)};
}

extern "C" void prompp_series_data_chunk_recoder_recode_next_chunk(void* args, void* res) {
  struct Arguments {
    entrypoint::ChunkRecoderPtr chunk_recoder;
  };
  struct Result {
    PromPP::Primitives::Timestamp min_t;
    PromPP::Primitives::Timestamp max_t;
    uint32_t series_id;
    uint8_t samples_count;
    bool has_more_data;
    PromPP::Primitives::Go::SliceView<uint8_t> buffer;
  };

  auto in = reinterpret_cast<Arguments*>(args);
  auto out = reinterpret_cast<Result*>(res);
  out->series_id = in->chunk_recoder->series_id();
  in->chunk_recoder->recode_next_chunk(*out);
  out->has_more_data = in->chunk_recoder->has_more_data();
  out->buffer.reset_to(in->chunk_recoder->bytes());
}

extern "C" void prompp_series_data_chunk_recoder_dtor(void* args) {
  struct Arguments {
    entrypoint::ChunkRecoderPtr chunk_recoder;
  };

  reinterpret_cast<Arguments*>(args)->chunk_recoder.~unique_ptr();
}