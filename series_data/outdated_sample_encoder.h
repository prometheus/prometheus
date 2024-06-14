#pragma once

#include "data_storage.h"

namespace series_data {

class OutdatedSampleEncoder {
 public:
  explicit OutdatedSampleEncoder(DataStorage& storage) : storage_(storage) {}

  void encode(uint32_t ls_id, int64_t timestamp, double value) {
    if (auto it = storage_.outdated_chunks_.try_emplace(ls_id, timestamp, value); !it.second) {
      it.first->second.encode(timestamp, value);
    }
  }

 private:
  DataStorage& storage_;
};

}  // namespace series_data