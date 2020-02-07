# Strfmt [![Build Status](https://travis-ci.org/go-openapi/strfmt.svg?branch=master)](https://travis-ci.org/go-openapi/strfmt) [![codecov](https://codecov.io/gh/go-openapi/strfmt/branch/master/graph/badge.svg)](https://codecov.io/gh/go-openapi/strfmt) [![Slack Status](https://slackin.goswagger.io/badge.svg)](https://slackin.goswagger.io)

[![license](http://img.shields.io/badge/license-Apache%20v2-orange.svg)](https://raw.githubusercontent.com/go-openapi/strfmt/master/LICENSE)
[![GoDoc](https://godoc.org/github.com/go-openapi/strfmt?status.svg)](http://godoc.org/github.com/go-openapi/strfmt)
[![GolangCI](https://golangci.com/badges/github.com/go-openapi/strfmt.svg)](https://golangci.com)
[![Go Report Card](https://goreportcard.com/badge/github.com/go-openapi/strfmt)](https://goreportcard.com/report/github.com/go-openapi/strfmt)

This package exposes a registry of data types to support string formats in the go-openapi toolkit.

strfmt represents a well known string format such as credit card or email. The go toolkit for OpenAPI specifications knows how to deal with those.

## Supported data formats
go-openapi/strfmt follows the swagger 2.0 specification with the following formats
defined [here](https://github.com/OAI/OpenAPI-Specification/blob/master/versions/2.0.md#data-types).

It also provides convenient extensions to go-openapi users.

- [x] JSON-schema draft 4 formats
  - date-time
  - email
  - hostname
  - ipv4
  - ipv6
  - uri
- [x] swagger 2.0 format extensions
  - binary
  - byte (e.g. base64 encoded string)
  - date (e.g. "1970-01-01")
  - password
- [x] go-openapi custom format extensions
  - bsonobjectid (BSON objectID)
  - creditcard
  - duration (e.g. "3 weeks", "1ms")
  - hexcolor (e.g. "#FFFFFF")
  - isbn, isbn10, isbn13
  - mac (e.g "01:02:03:04:05:06")
  - rgbcolor (e.g. "rgb(100,100,100)")
  - ssn
  - uuid, uuid3, uuid4, uuid5
  - cidr (e.g. "192.0.2.1/24", "2001:db8:a0b:12f0::1/32")

> NOTE: as the name stands for, this package is intended to support string formatting only.
> It does not provide validation for numerical values with swagger format extension for JSON types "number" or
> "integer" (e.g. float, double, int32...).

## Type conversion

All types defined here are stringers and may be converted to strings with `.String()`.
Note that most types defined by this package may be converted directly to string like `string(Email{})`.

`Date` and `DateTime` may be converted directly to `time.Time` like `time.Time(Time{})`.
Similarly, you can convert `Duration` to `time.Duration` as in `time.Duration(Duration{})`

## Using pointers

The `conv` subpackage provides helpers to convert the types to and from pointers, just like `go-openapi/swag` does
with primitive types.

## Format types
Types defined in strfmt expose marshaling and validation capabilities.

List of defined types:
- Base64
- CreditCard
- Date
- DateTime
- Duration
- Email
- HexColor
- Hostname
- IPv4
- IPv6
- CIDR
- ISBN
- ISBN10
- ISBN13
- MAC
- ObjectId
- Password
- RGBColor
- SSN
- URI
- UUID
- UUID3
- UUID4
- UUID5
