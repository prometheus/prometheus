#ifndef PROTOZERO_VERSION_HPP
#define PROTOZERO_VERSION_HPP

/*****************************************************************************

protozero - Minimalistic protocol buffer decoder and encoder in C++.

This file is from https://github.com/mapbox/protozero where you can find more
documentation.

*****************************************************************************/

/**
 * @file version.hpp
 *
 * @brief Contains macros defining the protozero version.
 */

/// The major version number
#define PROTOZERO_VERSION_MAJOR 1

/// The minor version number
#define PROTOZERO_VERSION_MINOR 7

/// The patch number
#define PROTOZERO_VERSION_PATCH 1

/// The complete version number
#define PROTOZERO_VERSION_CODE (PROTOZERO_VERSION_MAJOR * 10000 + PROTOZERO_VERSION_MINOR * 100 + PROTOZERO_VERSION_PATCH)

/// Version number as string
#define PROTOZERO_VERSION_STRING "1.7.1"

#endif // PROTOZERO_VERSION_HPP
