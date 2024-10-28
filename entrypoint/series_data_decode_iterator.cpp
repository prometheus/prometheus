#include "series_data/decoder/universal_decode_iterator.h"

extern "C" void prompp_series_data_decode_iterator_next(void* args, void* res) {
  using series_data::decoder::DecodeIteratorSentinel;
  using series_data::decoder::UniversalDecodeIterator;

  struct Arguments {
    UniversalDecodeIterator* decode_iterator;
  };

  using Result = struct {
    bool has_value;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  (*in->decode_iterator)++;
  out->has_value = (*in->decode_iterator) != DecodeIteratorSentinel{};
}

extern "C" void prompp_series_data_decode_iterator_sample(void* args, void* res) {
  using series_data::decoder::UniversalDecodeIterator;

  struct Arguments {
    UniversalDecodeIterator* decode_iterator;
  };
  using Result = struct {
    int64_t timestamp;
    double value;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  auto sample = **(in->decode_iterator);
  out->timestamp = sample.timestamp;
  out->value = sample.value;
}

extern "C" void prompp_series_data_decode_iterator_dtor(void* args) {
  using series_data::decoder::UniversalDecodeIterator;

  struct Arguments {
    UniversalDecodeIterator* decode_iterator;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->decode_iterator;
}
