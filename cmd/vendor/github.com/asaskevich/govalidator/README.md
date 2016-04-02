govalidator
===========
[![Gitter](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/asaskevich/govalidator?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge) [![GoDoc](https://godoc.org/github.com/asaskevich/govalidator?status.png)](https://godoc.org/github.com/asaskevich/govalidator) [![Coverage Status](https://img.shields.io/coveralls/asaskevich/govalidator.svg)](https://coveralls.io/r/asaskevich/govalidator?branch=master) [![views](https://sourcegraph.com/api/repos/github.com/asaskevich/govalidator/.counters/views.png)](https://sourcegraph.com/github.com/asaskevich/govalidator)
[![wercker status](https://app.wercker.com/status/1ec990b09ea86c910d5f08b0e02c6043/s "wercker status")](https://app.wercker.com/project/bykey/1ec990b09ea86c910d5f08b0e02c6043)
[![Build Status](https://travis-ci.org/asaskevich/govalidator.svg?branch=master)](https://travis-ci.org/asaskevich/govalidator)

A package of validators and sanitizers for strings, structs and collections. Based on [validator.js](https://github.com/chriso/validator.js).

#### Installation
Make sure that Go is installed on your computer.
Type the following command in your terminal:

	go get github.com/asaskevich/govalidator

After it the package is ready to use.

#### Import package in your project
Add following line in your `*.go` file:
```go
import "github.com/asaskevich/govalidator"
```
If you unhappy to use long `govalidator`, you can do something like this:
```go
import (
	valid "github.com/asaskevich/govalidator"
)
```

#### List of functions:
```go
func Abs(value float64) float64
func BlackList(str, chars string) string
func ByteLength(str string, params ...string) bool
func StringLength(str string, params ...string) bool
func StringMatches(s string, params ...string) bool
func CamelCaseToUnderscore(str string) string
func Contains(str, substring string) bool
func Count(array []interface{}, iterator ConditionIterator) int
func Each(array []interface{}, iterator Iterator)
func ErrorByField(e error, field string) string
func Filter(array []interface{}, iterator ConditionIterator) []interface{}
func Find(array []interface{}, iterator ConditionIterator) interface{}
func GetLine(s string, index int) (string, error)
func GetLines(s string) []string
func InRange(value, left, right float64) bool
func IsASCII(str string) bool
func IsAlpha(str string) bool
func IsAlphanumeric(str string) bool
func IsBase64(str string) bool
func IsByteLength(str string, min, max int) bool
func IsCreditCard(str string) bool
func IsDataURI(str string) bool
func IsDivisibleBy(str, num string) bool
func IsEmail(str string) bool
func IsFilePath(str string) (bool, int)
func IsFloat(str string) bool
func IsFullWidth(str string) bool
func IsHalfWidth(str string) bool
func IsHexadecimal(str string) bool
func IsHexcolor(str string) bool
func IsIP(str string) bool
func IsIPv4(str string) bool
func IsIPv6(str string) bool
func IsISBN(str string, version int) bool
func IsISBN10(str string) bool
func IsISBN13(str string) bool
func IsISO3166Alpha2(str string) bool
func IsISO3166Alpha3(str string) bool
func IsInt(str string) bool
func IsJSON(str string) bool
func IsLatitude(str string) bool
func IsLongitude(str string) bool
func IsLowerCase(str string) bool
func IsMAC(str string) bool
func IsMongoID(str string) bool
func IsMultibyte(str string) bool
func IsNatural(value float64) bool
func IsNegative(value float64) bool
func IsNonNegative(value float64) bool
func IsNonPositive(value float64) bool
func IsNull(str string) bool
func IsNumeric(str string) bool
func IsPositive(value float64) bool
func IsPrintableASCII(str string) bool
func IsRGBcolor(str string) bool
func IsRequestURI(rawurl string) bool
func IsRequestURL(rawurl string) bool
func IsSSN(str string) bool
func IsSemver(str string) bool
func IsURL(str string) bool
func IsUTFDigit(str string) bool
func IsUTFLetter(str string) bool
func IsUTFLetterNumeric(str string) bool
func IsUTFNumeric(str string) bool
func IsUUID(str string) bool
func IsUUIDv3(str string) bool
func IsUUIDv4(str string) bool
func IsUUIDv5(str string) bool
func IsUpperCase(str string) bool
func IsVariableWidth(str string) bool
func IsWhole(value float64) bool
func LeftTrim(str, chars string) string
func Map(array []interface{}, iterator ResultIterator) []interface{}
func Matches(str, pattern string) bool
func NormalizeEmail(str string) (string, error)
func RemoveTags(s string) string
func ReplacePattern(str, pattern, replace string) string
func Reverse(s string) string
func RightTrim(str, chars string) string
func SafeFileName(str string) string
func Sign(value float64) float64
func StripLow(str string, keepNewLines bool) string
func ToBoolean(str string) (bool, error)
func ToFloat(str string) (float64, error)
func ToInt(str string) (int64, error)
func ToJSON(obj interface{}) (string, error)
func ToString(obj interface{}) string
func Trim(str, chars string) string
func Truncate(str string, length int, ending string) string
func UnderscoreToCamelCase(s string) string
func ValidateStruct(s interface{}) (bool, error)
func WhiteList(str, chars string) string
type ConditionIterator
type Error
func (e Error) Error() string
type Errors
func (es Errors) Error() string
type ISO3166Entry
type Iterator
type ParamValidator
type ResultIterator
type UnsupportedTypeError
func (e *UnsupportedTypeError) Error() string
type Validator
```

#### Examples
###### IsURL
```go
println(govalidator.IsURL(`http://user@pass:domain.com/path/page`))
```
###### ToString
```go
type User struct {
	FirstName string
	LastName string
}

str, _ := govalidator.ToString(&User{"John", "Juan"})
println(str)
```
###### Each, Map, Filter, Count for slices
Each iterates over the slice/array and calls Iterator for every item
```go
data := []interface{}{1, 2, 3, 4, 5}
var fn govalidator.Iterator = func(value interface{}, index int) {
	println(value.(int))
}
govalidator.Each(data, fn)
```
```go
data := []interface{}{1, 2, 3, 4, 5}
var fn govalidator.ResultIterator = func(value interface{}, index int) interface{} {
	return value.(int) * 3
}
_ = govalidator.Map(data, fn) // result = []interface{}{1, 6, 9, 12, 15}
```
```go
data := []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
var fn govalidator.ConditionIterator = func(value interface{}, index int) bool {
	return value.(int)%2 == 0
}
_ = govalidator.Filter(data, fn) // result = []interface{}{2, 4, 6, 8, 10}
_ = govalidator.Count(data, fn) // result = 5
```
###### ValidateStruct [#2](https://github.com/asaskevich/govalidator/pull/2)
If you want to validate structs, you can use tag `valid` for any field in your structure. All validators used with this field in one tag are separated by comma. If you want to skip validation, place `-` in your tag. If you need a validator that is not on the list below, you can add it like this:
```go
govalidator.TagMap["duck"] = govalidator.Validator(func(str string) bool {
	return str == "duck"
})
```
Here is a list of available validators for struct fields (validator - used function):
```go
"alpha":          IsAlpha,
"alphanum":       IsAlphanumeric,
"ascii":          IsASCII,
"base64":         IsBase64,
"creditcard":     IsCreditCard,
"datauri":        IsDataURI,
"email":          IsEmail,
"float":          IsFloat,
"fullwidth":      IsFullWidth,
"halfwidth":      IsHalfWidth,
"hexadecimal":    IsHexadecimal,
"hexcolor":       IsHexcolor,
"int":            IsInt,
"ip":             IsIP,
"ipv4":           IsIPv4,
"ipv6":           IsIPv6,
"isbn10":         IsISBN10,
"isbn13":         IsISBN13,
"json":           IsJSON,
"latitude":       IsLatitude,
"longitude":      IsLongitude,
"lowercase":      IsLowerCase,
"mac":            IsMAC,
"multibyte":      IsMultibyte,
"null":           IsNull,
"numeric":        IsNumeric,
"printableascii": IsPrintableASCII,
"requri":         IsRequestURI,
"requrl":         IsRequestURL,
"rgbcolor":       IsRGBcolor,
"ssn":            IsSSN,
"semver":         IsSemver,
"uppercase":      IsUpperCase,
"url":            IsURL,
"utfdigit":       IsUTFDigit,
"utfletter":      IsUTFLetter,
"utfletternum":   IsUTFLetterNumeric,
"utfnumeric":     IsUTFNumeric,
"uuid":           IsUUID,
"uuidv3":         IsUUIDv3,
"uuidv4":         IsUUIDv4,
"uuidv5":         IsUUIDv5,
"variablewidth":  IsVariableWidth,
```
Validators with parameters

```go
"length(min|max)": ByteLength,
"matches(pattern)": StringMatches,
```

And here is small example of usage:
```go
type Post struct {
	Title    string `valid:"alphanum,required"`
	Message  string `valid:"duck,ascii"`
	AuthorIP string `valid:"ipv4"`
	Date     string `valid:"-"`
}
post := &Post{
	Title:   "My Example Post",
	Message: "duck",
	AuthorIP: "123.234.54.3",
}

// Add your own struct validation tags
govalidator.TagMap["duck"] = govalidator.Validator(func(str string) bool {
	return str == "duck"
})

result, err := govalidator.ValidateStruct(post)
if err != nil {
	println("error: " + err.Error())
}
println(result)
```
###### WhiteList
```go
// Remove all characters from string ignoring characters between "a" and "z"
println(govalidator.WhiteList("a3a43a5a4a3a2a23a4a5a4a3a4", "a-z") == "aaaaaaaaaaaa")
```

#### Notes
Documentation is available here: [godoc.org](https://godoc.org/github.com/asaskevich/govalidator).
Full information about code coverage is also available here: [govalidator on gocover.io](http://gocover.io/github.com/asaskevich/govalidator).

#### Support
If you do have a contribution for the package feel free to put up a Pull Request or open Issue.

#### Special thanks to [contributors](https://github.com/asaskevich/govalidator/graphs/contributors)
* [Attila Oláh](https://github.com/attilaolah)
* [Daniel Korner](https://github.com/Dadie)
* [Steven Wilkin](https://github.com/stevenwilkin)
* [Deiwin Sarjas](https://github.com/deiwin)
* [Noah Shibley](https://github.com/slugmobile)
* [Nathan Davies](https://github.com/nathj07)
* [Matt Sanford](https://github.com/mzsanford)
* [Simon ccl1115](https://github.com/ccl1115)

[![Bitdeli Badge](https://d2weczhvl823v0.cloudfront.net/asaskevich/govalidator/trend.png)](https://bitdeli.com/free "Bitdeli Badge")

