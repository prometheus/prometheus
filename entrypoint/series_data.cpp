#include "series_data.h"
#include "series_data/data_storage.h"
#include "series_data/encoder.h"
#include "series_data/outdated_sample_encoder.h"
#include <chrono>

extern "C" void prompp_series_data_data_storage_ctor(void* res) {
  using Result = struct {
    series_data::DataStorage* data_storage;
  };

  Result* out = new (res) Result();
  out->data_storage = new series_data::DataStorage();
}

extern "C" void prompp_series_data_data_storage_dtor(void* args) {
  struct Arguments {
    series_data::DataStorage* data_storage;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->data_storage;
}

struct SeriesDataEncoderWrapper {
    std::chrono::system_clock clock{};
    series_data::OutdatedSampleEncoder<std::chrono::system_clock> outdated_sample_encoder;
    series_data::Encoder<decltype(outdated_sample_encoder)> encoder;

    SeriesDataEncoderWrapper(series_data::DataStorage& data_storage) : outdated_sample_encoder{data_storage, clock}, encoder{data_storage, outdated_sample_encoder} {}
};

extern "C" void prompp_series_data_encoder_ctor(void* args, void* res) {
  struct Arguments {
    series_data::DataStorage* data_storage;
  };
  using Result = struct {
    SeriesDataEncoderWrapper* encoder_wrapper;
  };

  auto* in = reinterpret_cast<Arguments*>(args);
  new (res) Result{.encoder_wrapper = new SeriesDataEncoderWrapper(*in->data_storage)};
}

extern "C" void prompp_series_data_encoder_dtor(void* args) {
  struct Arguments {
    SeriesDataEncoderWrapper* encoder_wrapper;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->encoder_wrapper;
}