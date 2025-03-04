#pragma once

namespace PromPP::WAL::concepts {

template <class T>
concept has_field_segment_id = requires(const T& t) {
  { t.segment_id };
};

}  // namespace PromPP::WAL::concepts
