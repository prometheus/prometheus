#include "series_data_deserializer.h"
#include "primitives/go_slice.h"
#include "series_data/serialization/deserializer.h"

extern "C" void prompp_series_data_deserializer_ctor(void* args, void* res) {
  using PromPP::Primitives::Go::SliceView;
  using series_data::serialization::Deserializer;

  struct Arguments {
    SliceView<uint8_t> chunk_data;
  };

  using Result = struct {
    Deserializer* deserializer;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  std::span<const uint8_t> chunk_span(in->chunk_data.data(), in->chunk_data.size());
  out->deserializer = new series_data::serialization::Deserializer(chunk_span);
}

extern "C" void prompp_series_data_deserializer_create_decode_iterator(void* args, void* res) {
  using PromPP::Primitives::Go::Slice;
  using series_data::chunk::SerializedChunk;
  using series_data::decoder::UniversalDecodeIterator;
  using series_data::serialization::Deserializer;

  struct Arguments {
    Deserializer* deserializer;
    Slice<uint8_t> chunk_metadata;
  };

  using Result = struct {
    UniversalDecodeIterator* decode_iterator;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  SerializedChunk* serialized_chunk = reinterpret_cast<SerializedChunk*>(in->chunk_metadata.data());
  out->decode_iterator = new UniversalDecodeIterator{in->deserializer->create_decode_iterator(*serialized_chunk)};
}

extern "C" void prompp_series_data_deserializer_dtor(void* args) {
  using series_data::serialization::Deserializer;
  struct Arguments {
    Deserializer* deserializer;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->deserializer;
}
