#include "series_data_data_storage.h"

#include "head/chunk_recoder.h"
#include "head/data_storage.h"
#include "primitives/go_slice.h"
#include "series_data/data_storage.h"
#include "series_data/querier/querier.h"
#include "series_data/serialization/serializer.h"

using entrypoint::head::DataStoragePtr;
using ChunkRecoderPtr = std::unique_ptr<head::ChunkRecoder>;

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

  static_cast<Arguments*>(args)->data_storage->reset();
}

extern "C" void prompp_series_data_data_storage_time_interval(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
  };
  struct Result {
    PromPP::Primitives::TimeInterval interval;
  };

  new (res) Result{.interval = series_data::Decoder::get_time_interval(*static_cast<Arguments*>(args)->data_storage)};
}

extern "C" void prompp_series_data_data_storage_query(void* args, void* res) {
  using PromPP::Primitives::LabelSetID;
  using PromPP::Primitives::Go::Slice;
  using series_data::DataStorage;
  using Query = series_data::querier::Query<Slice<LabelSetID>>;
  using PromPP::Primitives::Go::BytesStream;
  using series_data::querier::Querier;
  using series_data::serialization::Serializer;

  struct Arguments {
    DataStorage* data_storage;
    Query query;
  };

  using Result = struct {
    Slice<char> serialized_chunks;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  Querier querier{*in->data_storage};
  const auto& queried_chunk_list = querier.query(in->query);
  Serializer serializer{*in->data_storage};
  BytesStream bytes_stream{&out->serialized_chunks};
  serializer.serialize(queried_chunk_list, bytes_stream);
}

extern "C" void prompp_series_data_data_storage_allocated_memory(void* args, void* res) {
  using series_data::DataStorage;

  struct Arguments {
    DataStorage* data_storage;
  };

  struct Result {
    uint64_t allocated_memory;
  };

  auto in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  out->allocated_memory = in->data_storage->allocated_memory();
}

extern "C" void prompp_series_data_data_storage_dtor(void* args) {
  struct Arguments {
    DataStoragePtr data_storage;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_series_data_chunk_recoder_ctor(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
    PromPP::Primitives::TimeInterval time_interval;
  };
  struct Result {
    ChunkRecoderPtr chunk_recoder;
  };

  const auto in = static_cast<Arguments*>(args);
  new (res) Result{.chunk_recoder = std::make_unique<head::ChunkRecoder>(in->data_storage.get(), in->time_interval)};
}

extern "C" void prompp_series_data_chunk_recoder_recode_next_chunk(void* args, void* res) {
  struct Arguments {
    ChunkRecoderPtr chunk_recoder;
  };
  struct Result {
    PromPP::Primitives::TimeInterval interval;
    uint32_t series_id;
    uint8_t samples_count;
    bool has_more_data;
    PromPP::Primitives::Go::SliceView<uint8_t> buffer;
  };

  const auto in = static_cast<const Arguments*>(args);
  const auto out = static_cast<Result*>(res);
  in->chunk_recoder->recode_next_chunk(*out);
  out->has_more_data = in->chunk_recoder->has_more_data();
  out->buffer.reset_to(in->chunk_recoder->bytes());
}

extern "C" void prompp_series_data_chunk_recoder_dtor(void* args) {
  struct Arguments {
    ChunkRecoderPtr chunk_recoder;
  };

  static_cast<Arguments*>(args)->~Arguments();
}
