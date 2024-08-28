#include "series_data_data_storage.h"
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