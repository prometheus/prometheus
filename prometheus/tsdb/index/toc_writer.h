#pragma once

#include "bare_bones/preprocess.h"
#include "stream_writer.h"
#include "toc.h"

namespace PromPP::Prometheus::tsdb::index {

class TocWriter {
 public:
  TocWriter(const Toc& toc, StreamWriter& writer) : toc_(toc), writer_(writer) {}

  void write() {
    Toc toc{
        .symbols = BareBones::Bit::be(toc_.symbols),
        .series = BareBones::Bit::be(toc_.series),
        .label_indices = BareBones::Bit::be(toc_.label_indices),
        .label_indices_table = BareBones::Bit::be(toc_.label_indices_table),
        .postings = BareBones::Bit::be(toc_.postings),
        .postings_offset_table = BareBones::Bit::be(toc_.postings_offset_table),
    };

    std::string_view toc_view{reinterpret_cast<const char*>(&toc), sizeof(toc)};
    writer_.write(toc_view);
    writer_.compute_and_write_crc32(toc_view);
  }

 private:
  const Toc& toc_;
  StreamWriter& writer_;
};

}  // namespace PromPP::Prometheus::tsdb::index