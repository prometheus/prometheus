#pragma once

#include "go_slice.h"

#include "third_party/protozero/buffer_tmpl.hpp"
#include "third_party/protozero/config.hpp"

namespace protozero {

template <>
struct buffer_customization<PromPP::Primitives::Go::Slice<char>> {
  static std::size_t size(const PromPP::Primitives::Go::Slice<char>* buffer) noexcept { return buffer->size(); }

  static void append(PromPP::Primitives::Go::Slice<char>* buffer, const char* data, std::size_t count) { buffer->push_back(data, data + count); }

  static void append_zeros(PromPP::Primitives::Go::Slice<char>* buffer, std::size_t count) { buffer->push_back(count, '\0'); }

  static void resize(PromPP::Primitives::Go::Slice<char>* buffer, std::size_t size) {
    protozero_assert(size < buffer->size());
    buffer->resize(size);
  }

  static void reserve_additional(PromPP::Primitives::Go::Slice<char>* buffer, std::size_t size) { buffer->reserve(buffer->size() + size); }

  static void erase_range(PromPP::Primitives::Go::Slice<char>* buffer, std::size_t from, std::size_t to) {
    protozero_assert(from <= buffer->size());
    protozero_assert(to <= buffer->size());
    protozero_assert(from <= to);
    buffer->erase(std::next(buffer->begin(), static_cast<std::string::iterator::difference_type>(from)),
                  std::next(buffer->begin(), static_cast<std::string::iterator::difference_type>(to)));
  }

  static char* at_pos(PromPP::Primitives::Go::Slice<char>* buffer, std::size_t pos) {
    protozero_assert(pos <= buffer->size());
    return (&*buffer->begin()) + pos;
  }

  static void push_back(PromPP::Primitives::Go::Slice<char>* buffer, char ch) { buffer->push_back(ch); }
};

}  // namespace protozero
