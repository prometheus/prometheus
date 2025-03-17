#pragma once

#include <variant>

#include "wal/hashdex/basic_decoder.h"
#include "wal/hashdex/go_model.h"
#include "wal/hashdex/protobuf.h"
#include "wal/hashdex/scraper/scraper.h"

/**
 * used for indexing HashdexVariant.
 */
enum HashdexType : uint8_t {
  kProtobuf = 0,
  kGoModel,
  kDecoder,
  kPrometheusScraper,
  kOpenMetricsScraper,
};

using HashdexVariant = std::variant<PromPP::WAL::hashdex::Protobuf,
                                    PromPP::WAL::hashdex::GoModel,
                                    PromPP::WAL::hashdex::BasicDecoder,
                                    PromPP::WAL::hashdex::scraper::PrometheusScraper,
                                    PromPP::WAL::hashdex::scraper::OpenMetricsScraper>;
