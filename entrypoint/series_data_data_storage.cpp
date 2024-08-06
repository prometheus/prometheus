#include "series_data_data_storage.h"
#include "series_data/data_storage.h"

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