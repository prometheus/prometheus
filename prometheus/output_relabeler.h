#pragma once

#include "primitives/snug_composites.h"
#include "prometheus/stateless_relabeler.h"

namespace PromPP::Prometheus::Relabel {
class WALOutputDecoder {
  PromPP::Primitives::SnugComposites::LabelSet::EncodingTable wal_lss_;
  StatelessRelabeler* stateless_relabeler_ [[maybe_unused]];
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap output_lss_;
  //
 public:
};
}  // namespace PromPP::Prometheus::Relabel
