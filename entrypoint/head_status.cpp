#include "head_status.h"

#include "head/data_storage.h"
#include "head/lss.h"
#include "head/status.h"
#include "primitives/go_slice.h"

using entrypoint::head::DataStoragePtr;
using entrypoint::head::LssVariantPtr;

using Status = head::Status<PromPP::Primitives::Go::String, PromPP::Primitives::Go::Slice>;

extern "C" void prompp_get_head_status(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    DataStoragePtr data_storage;
    size_t limit;
  };

  const auto in = static_cast<const Arguments*>(args);
  const auto& lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->lss);

  head::StatusGetter<entrypoint::head::QueryableEncodingBimap, Status>{lss, *in->data_storage, in->limit}.get(*static_cast<Status*>(res));
}

extern "C" void prompp_free_head_status(void* args) {
  static_cast<Status*>(args)->~Status();
}
