#ifndef PROTOZERO_PBF_BUILDER_HPP
#define PROTOZERO_PBF_BUILDER_HPP

/*****************************************************************************

protozero - Minimalistic protocol buffer decoder and encoder in C++.

This file is from https://github.com/mapbox/protozero where you can find more
documentation.

*****************************************************************************/

/**
 * @file pbf_builder.hpp
 *
 * @brief Contains the pbf_builder template class.
 */

#include "basic_pbf_builder.hpp"
#include "pbf_writer.hpp"

#include <string>

namespace protozero {

/// Specialization of basic_pbf_builder using std::string as buffer type.
template <typename T>
using pbf_builder = basic_pbf_builder<std::string, T>;

} // end namespace protozero

#endif // PROTOZERO_PBF_BUILDER_HPP
