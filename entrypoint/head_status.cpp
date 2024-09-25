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
  };

  static constexpr size_t kTopItemsCount = 10;

  const auto in = static_cast<const Arguments*>(args);
  const auto& lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->lss);

  head::StatusGetter<entrypoint::head::QueryableEncodingBimap, Status, kTopItemsCount>{lss, *in->data_storage}.get(*static_cast<Status*>(res));
}

void prompp_free_head_status(void* args) {
  const auto in = static_cast<Status*>(args);

  in->label_value_count_by_label_name.free();
  in->series_count_by_metric_name.free();
  in->memory_in_bytes_by_label_name.free();
  in->series_count_by_label_value_pair.free();
}
