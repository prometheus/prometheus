#pragma once

#include "bare_bones/preprocess.h"
#include "stream_writer.h"
#include "toc.h"

namespace PromPP::Prometheus::tsdb::index {

template <class Stream>
class TocWriter {
 public:
  TocWriter(const Toc& toc, StreamWriter<Stream>& writer) : toc_(toc), writer_(writer) {}

  void write() {
    Toc toc{
        .symbols = BareBones::Bit::be(toc_.symbols),
        .series = BareBones::Bit::be(toc_.series),
        .label_indices = BareBones::Bit::be(toc_.label_indices),
        .label_indices_table = BareBones::Bit::be(toc_.label_indices_table),
        .postings = BareBones::Bit::be(toc_.postings),
        .postings_offset_table = BareBones::Bit::be(toc_.postings_offset_table),
    };

    writer_.reset_crc32();
    writer_.write({reinterpret_cast<const char*>(&toc), sizeof(toc)});
    writer_.write_crc32();
  }

 private:
  const Toc& toc_;
  StreamWriter<Stream>& writer_;
};

}  // namespace PromPP::Prometheus::tsdb::index