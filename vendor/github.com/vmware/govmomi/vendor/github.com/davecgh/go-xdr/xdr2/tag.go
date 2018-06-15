/*
 * Copyright (c) 2015-2017 Giovanni Bajo <rasky@develer.com>
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package xdr

import (
	"reflect"
	"strings"
)

// xdrtag represents a XDR struct tag, identified by the name "xdr:".
// The value of the tag is a string that is parsed as a comma-separated
// list of =-separated key-value options. If an option has no value,
// "true" is assumed to be the default value.
//
// For instance:
//
//    `xdr:"foo,bar=2,baz=false"
//
// After parsing this tag, Get("foo") will return "true", Get("bar")
// will return "2", and Get("baz") will return "false".
type xdrtag string

// parseTag extracts a xdrtag from the original reflect.StructTag as found in
// in the struct field. If the tag was not specified, an empty strtag is
// returned.
func parseTag(tag reflect.StructTag) xdrtag {
	t := tag.Get("xdr")
	// Handle backward compatibility with the previous "xdropaque"
	// tag which is now deprecated.
	if tag.Get("xdropaque") == "false" {
		if t == "" {
			t = ","
		}
		t += ",opaque=false"
	}
	return xdrtag(t)
}

// Get returns the value for the specified option. If the option is not
// present in the tag, an empty string is returned. If the option is
// present but has no value, the string "true" is returned as default value.
func (t xdrtag) Get(opt string) string {
	tag := string(t)
	for tag != "" {
		var next string
		i := strings.Index(tag, ",")
		if i >= 0 {
			tag, next = tag[:i], tag[i+1:]
		}
		if tag == opt {
			return "true"
		}
		if len(tag) > len(opt) && tag[:len(opt)] == opt && tag[len(opt)] == '=' {
			return tag[len(opt)+1:]
		}
		tag = next
	}
	return ""
}
