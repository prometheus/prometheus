#pragma once

#include <iostream>
#include <span>

#include <scope_exit.h>

namespace BareBones {

inline constinit uint8_t kSerializerVersion = 1;

template <class T>
void serialize(std::ostream& stream, std::span<T> items) {
  auto sg1 = std::experimental::scope_exit([original_exceptions = stream.exceptions(), &stream]() { stream.exceptions(original_exceptions); });
  stream.exceptions(std::ifstream::failbit | std::ifstream::badbit);

  stream.put(kSerializerVersion);

  const auto count = static_cast<uint32_t>(items.size());
  stream.write(reinterpret_cast<const char*>(&count), sizeof(count));

  if (count > 0) [[likely]] {
    stream.write(reinterpret_cast<const char*>(items.data()), sizeof(T) * count);
  }
}

template <class Vector>
void deserialize(std::istream& stream, Vector& vector) {
  auto vector_size = static_cast<uint32_t>(vector.size());
  auto sg1 = std::experimental::scope_fail([vector_size, &vector] { vector.resize(vector_size); });

  if (const uint8_t version = stream.get(); stream.eof()) {
    return;
  } else if (version != kSerializerVersion) {
    throw Exception(0xed3acf7e0d0327d2, "Invalid serializer version %d got from input stream, only version %d is supported", version, kSerializerVersion);
  }

  auto sg2 = std::experimental::scope_exit([original_exceptions = stream.exceptions(), &stream] { stream.exceptions(original_exceptions); });
  stream.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit);

  uint32_t count{0};
  stream.read(reinterpret_cast<char*>(&count), sizeof(count));

  if (count > 0) [[likely]] {
    vector.resize(vector_size + count);
    stream.read(reinterpret_cast<char*>(&vector[vector_size]), sizeof(uint32_t) * count);
  }
}

}  // namespace BareBones