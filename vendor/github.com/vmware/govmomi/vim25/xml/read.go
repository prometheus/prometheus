// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xml

import (
	"bytes"
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// BUG(rsc): Mapping between XML elements and data structures is inherently flawed:
// an XML element is an order-dependent collection of anonymous
// values, while a data structure is an order-independent collection
// of named values.
// See package json for a textual representation more suitable
// to data structures.

// Unmarshal parses the XML-encoded data and stores the result in
// the value pointed to by v, which must be an arbitrary struct,
// slice, or string. Well-formed data that does not fit into v is
// discarded.
//
// Because Unmarshal uses the reflect package, it can only assign
// to exported (upper case) fields.  Unmarshal uses a case-sensitive
// comparison to match XML element names to tag values and struct
// field names.
//
// Unmarshal maps an XML element to a struct using the following rules.
// In the rules, the tag of a field refers to the value associated with the
// key 'xml' in the struct field's tag (see the example above).
//
//   * If the struct has a field of type []byte or string with tag
//      ",innerxml", Unmarshal accumulates the raw XML nested inside the
//      element in that field.  The rest of the rules still apply.
//
//   * If the struct has a field named XMLName of type xml.Name,
//      Unmarshal records the element name in that field.
//
//   * If the XMLName field has an associated tag of the form
//      "name" or "namespace-URL name", the XML element must have
//      the given name (and, optionally, name space) or else Unmarshal
//      returns an error.
//
//   * If the XML element has an attribute whose name matches a
//      struct field name with an associated tag containing ",attr" or
//      the explicit name in a struct field tag of the form "name,attr",
//      Unmarshal records the attribute value in that field.
//
//   * If the XML element contains character data, that data is
//      accumulated in the first struct field that has tag ",chardata".
//      The struct field may have type []byte or string.
//      If there is no such field, the character data is discarded.
//
//   * If the XML element contains comments, they are accumulated in
//      the first struct field that has tag ",comment".  The struct
//      field may have type []byte or string.  If there is no such
//      field, the comments are discarded.
//
//   * If the XML element contains a sub-element whose name matches
//      the prefix of a tag formatted as "a" or "a>b>c", unmarshal
//      will descend into the XML structure looking for elements with the
//      given names, and will map the innermost elements to that struct
//      field. A tag starting with ">" is equivalent to one starting
//      with the field name followed by ">".
//
//   * If the XML element contains a sub-element whose name matches
//      a struct field's XMLName tag and the struct field has no
//      explicit name tag as per the previous rule, unmarshal maps
//      the sub-element to that struct field.
//
//   * If the XML element contains a sub-element whose name matches a
//      field without any mode flags (",attr", ",chardata", etc), Unmarshal
//      maps the sub-element to that struct field.
//
//   * If the XML element contains a sub-element that hasn't matched any
//      of the above rules and the struct has a field with tag ",any",
//      unmarshal maps the sub-element to that struct field.
//
//   * An anonymous struct field is handled as if the fields of its
//      value were part of the outer struct.
//
//   * A struct field with tag "-" is never unmarshalled into.
//
// Unmarshal maps an XML element to a string or []byte by saving the
// concatenation of that element's character data in the string or
// []byte. The saved []byte is never nil.
//
// Unmarshal maps an attribute value to a string or []byte by saving
// the value in the string or slice.
//
// Unmarshal maps an XML element to a slice by extending the length of
// the slice and mapping the element to the newly created value.
//
// Unmarshal maps an XML element or attribute value to a bool by
// setting it to the boolean value represented by the string.
//
// Unmarshal maps an XML element or attribute value to an integer or
// floating-point field by setting the field to the result of
// interpreting the string value in decimal.  There is no check for
// overflow.
//
// Unmarshal maps an XML element to an xml.Name by recording the
// element name.
//
// Unmarshal maps an XML element to a pointer by setting the pointer
// to a freshly allocated value and then mapping the element to that value.
//
func Unmarshal(data []byte, v interface{}) error {
	return NewDecoder(bytes.NewReader(data)).Decode(v)
}

// Decode works like xml.Unmarshal, except it reads the decoder
// stream to find the start element.
func (d *Decoder) Decode(v interface{}) error {
	return d.DecodeElement(v, nil)
}

// DecodeElement works like xml.Unmarshal except that it takes
// a pointer to the start XML element to decode into v.
// It is useful when a client reads some raw XML tokens itself
// but also wants to defer to Unmarshal for some elements.
func (d *Decoder) DecodeElement(v interface{}, start *StartElement) error {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Ptr {
		return errors.New("non-pointer passed to Unmarshal")
	}
	return d.unmarshal(val.Elem(), start)
}

// An UnmarshalError represents an error in the unmarshalling process.
type UnmarshalError string

func (e UnmarshalError) Error() string { return string(e) }

// Unmarshaler is the interface implemented by objects that can unmarshal
// an XML element description of themselves.
//
// UnmarshalXML decodes a single XML element
// beginning with the given start element.
// If it returns an error, the outer call to Unmarshal stops and
// returns that error.
// UnmarshalXML must consume exactly one XML element.
// One common implementation strategy is to unmarshal into
// a separate value with a layout matching the expected XML
// using d.DecodeElement,  and then to copy the data from
// that value into the receiver.
// Another common strategy is to use d.Token to process the
// XML object one token at a time.
// UnmarshalXML may not use d.RawToken.
type Unmarshaler interface {
	UnmarshalXML(d *Decoder, start StartElement) error
}

// UnmarshalerAttr is the interface implemented by objects that can unmarshal
// an XML attribute description of themselves.
//
// UnmarshalXMLAttr decodes a single XML attribute.
// If it returns an error, the outer call to Unmarshal stops and
// returns that error.
// UnmarshalXMLAttr is used only for struct fields with the
// "attr" option in the field tag.
type UnmarshalerAttr interface {
	UnmarshalXMLAttr(attr Attr) error
}

// receiverType returns the receiver type to use in an expression like "%s.MethodName".
func receiverType(val interface{}) string {
	t := reflect.TypeOf(val)
	if t.Name() != "" {
		return t.String()
	}
	return "(" + t.String() + ")"
}

// unmarshalInterface unmarshals a single XML element into val.
// start is the opening tag of the element.
func (p *Decoder) unmarshalInterface(val Unmarshaler, start *StartElement) error {
	// Record that decoder must stop at end tag corresponding to start.
	p.pushEOF()

	p.unmarshalDepth++
	err := val.UnmarshalXML(p, *start)
	p.unmarshalDepth--
	if err != nil {
		p.popEOF()
		return err
	}

	if !p.popEOF() {
		return fmt.Errorf("xml: %s.UnmarshalXML did not consume entire <%s> element", receiverType(val), start.Name.Local)
	}

	return nil
}

// unmarshalTextInterface unmarshals a single XML element into val.
// The chardata contained in the element (but not its children)
// is passed to the text unmarshaler.
func (p *Decoder) unmarshalTextInterface(val encoding.TextUnmarshaler, start *StartElement) error {
	var buf []byte
	depth := 1
	for depth > 0 {
		t, err := p.Token()
		if err != nil {
			return err
		}
		switch t := t.(type) {
		case CharData:
			if depth == 1 {
				buf = append(buf, t...)
			}
		case StartElement:
			depth++
		case EndElement:
			depth--
		}
	}
	return val.UnmarshalText(buf)
}

// unmarshalAttr unmarshals a single XML attribute into val.
func (p *Decoder) unmarshalAttr(val reflect.Value, attr Attr) error {
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			val.Set(reflect.New(val.Type().Elem()))
		}
		val = val.Elem()
	}

	if val.CanInterface() && val.Type().Implements(unmarshalerAttrType) {
		// This is an unmarshaler with a non-pointer receiver,
		// so it's likely to be incorrect, but we do what we're told.
		return val.Interface().(UnmarshalerAttr).UnmarshalXMLAttr(attr)
	}
	if val.CanAddr() {
		pv := val.Addr()
		if pv.CanInterface() && pv.Type().Implements(unmarshalerAttrType) {
			return pv.Interface().(UnmarshalerAttr).UnmarshalXMLAttr(attr)
		}
	}

	// Not an UnmarshalerAttr; try encoding.TextUnmarshaler.
	if val.CanInterface() && val.Type().Implements(textUnmarshalerType) {
		// This is an unmarshaler with a non-pointer receiver,
		// so it's likely to be incorrect, but we do what we're told.
		return val.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(attr.Value))
	}
	if val.CanAddr() {
		pv := val.Addr()
		if pv.CanInterface() && pv.Type().Implements(textUnmarshalerType) {
			return pv.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(attr.Value))
		}
	}

	copyValue(val, []byte(attr.Value))
	return nil
}

var (
	unmarshalerType     = reflect.TypeOf((*Unmarshaler)(nil)).Elem()
	unmarshalerAttrType = reflect.TypeOf((*UnmarshalerAttr)(nil)).Elem()
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

// Find reflect.Type for an element's type attribute.
func (p *Decoder) typeForElement(val reflect.Value, start *StartElement) reflect.Type {
	t := ""
	for i, a := range start.Attr {
		if a.Name == xmlSchemaInstance || a.Name == xsiType {
			t = a.Value
			// HACK: ensure xsi:type is last in the list to avoid using that value for
			// a "type" attribute, such as ManagedObjectReference.Type for example.
			// Note that xsi:type is already the last attribute in VC/ESX responses.
			// This is only an issue with govmomi simulator generated responses.
			// Proper fix will require finding a few needles in this xml package haystack.
			// Note: govmomi uses xmlSchemaInstance, other clients (e.g. rbvmomi) use xsiType.
			// They are the same thing to XML parsers, but not to this hack here.
			x := len(start.Attr) - 1
			if i != x {
				start.Attr[i] = start.Attr[x]
				start.Attr[x] = a
			}
			break
		}
	}

	if t == "" {
		// No type attribute; fall back to looking up type by interface name.
		t = val.Type().Name()
	}

	// Maybe the type is a basic xsd:* type.
	typ := stringToType(t)
	if typ != nil {
		return typ
	}

	// Maybe the type is a custom type.
	if p.TypeFunc != nil {
		if typ, ok := p.TypeFunc(t); ok {
			return typ
		}
	}

	return nil
}

// Unmarshal a single XML element into val.
func (p *Decoder) unmarshal(val reflect.Value, start *StartElement) error {
	// Find start element if we need it.
	if start == nil {
		for {
			tok, err := p.Token()
			if err != nil {
				return err
			}
			if t, ok := tok.(StartElement); ok {
				start = &t
				break
			}
		}
	}

	// Try to figure out type for empty interface values.
	if val.Kind() == reflect.Interface && val.IsNil() {
		typ := p.typeForElement(val, start)
		if typ != nil {
			pval := reflect.New(typ).Elem()
			err := p.unmarshal(pval, start)
			if err != nil {
				return err
			}

			for i := 0; i < 2; i++ {
				if typ.Implements(val.Type()) {
					val.Set(pval)
					return nil
				}

				typ = reflect.PtrTo(typ)
				pval = pval.Addr()
			}

			val.Set(pval)
			return nil
		}
	}

	// Load value from interface, but only if the result will be
	// usefully addressable.
	if val.Kind() == reflect.Interface && !val.IsNil() {
		e := val.Elem()
		if e.Kind() == reflect.Ptr && !e.IsNil() {
			val = e
		}
	}

	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			val.Set(reflect.New(val.Type().Elem()))
		}
		val = val.Elem()
	}

	if val.CanInterface() && val.Type().Implements(unmarshalerType) {
		// This is an unmarshaler with a non-pointer receiver,
		// so it's likely to be incorrect, but we do what we're told.
		return p.unmarshalInterface(val.Interface().(Unmarshaler), start)
	}

	if val.CanAddr() {
		pv := val.Addr()
		if pv.CanInterface() && pv.Type().Implements(unmarshalerType) {
			return p.unmarshalInterface(pv.Interface().(Unmarshaler), start)
		}
	}

	if val.CanInterface() && val.Type().Implements(textUnmarshalerType) {
		return p.unmarshalTextInterface(val.Interface().(encoding.TextUnmarshaler), start)
	}

	if val.CanAddr() {
		pv := val.Addr()
		if pv.CanInterface() && pv.Type().Implements(textUnmarshalerType) {
			return p.unmarshalTextInterface(pv.Interface().(encoding.TextUnmarshaler), start)
		}
	}

	var (
		data         []byte
		saveData     reflect.Value
		comment      []byte
		saveComment  reflect.Value
		saveXML      reflect.Value
		saveXMLIndex int
		saveXMLData  []byte
		saveAny      reflect.Value
		sv           reflect.Value
		tinfo        *typeInfo
		err          error
	)

	switch v := val; v.Kind() {
	default:
		return errors.New("unknown type " + v.Type().String())

	case reflect.Interface:
		// TODO: For now, simply ignore the field. In the near
		//       future we may choose to unmarshal the start
		//       element on it, if not nil.
		return p.Skip()

	case reflect.Slice:
		typ := v.Type()
		if typ.Elem().Kind() == reflect.Uint8 {
			// []byte
			saveData = v
			break
		}

		// Slice of element values.
		// Grow slice.
		n := v.Len()
		if n >= v.Cap() {
			ncap := 2 * n
			if ncap < 4 {
				ncap = 4
			}
			new := reflect.MakeSlice(typ, n, ncap)
			reflect.Copy(new, v)
			v.Set(new)
		}
		v.SetLen(n + 1)

		// Recur to read element into slice.
		if err := p.unmarshal(v.Index(n), start); err != nil {
			v.SetLen(n)
			return err
		}
		return nil

	case reflect.Bool, reflect.Float32, reflect.Float64, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.String:
		saveData = v

	case reflect.Struct:
		typ := v.Type()
		if typ == nameType {
			v.Set(reflect.ValueOf(start.Name))
			break
		}

		sv = v
		tinfo, err = getTypeInfo(typ)
		if err != nil {
			return err
		}

		// Validate and assign element name.
		if tinfo.xmlname != nil {
			finfo := tinfo.xmlname
			if finfo.name != "" && finfo.name != start.Name.Local {
				return UnmarshalError("expected element type <" + finfo.name + "> but have <" + start.Name.Local + ">")
			}
			if finfo.xmlns != "" && finfo.xmlns != start.Name.Space {
				e := "expected element <" + finfo.name + "> in name space " + finfo.xmlns + " but have "
				if start.Name.Space == "" {
					e += "no name space"
				} else {
					e += start.Name.Space
				}
				return UnmarshalError(e)
			}
			fv := finfo.value(sv)
			if _, ok := fv.Interface().(Name); ok {
				fv.Set(reflect.ValueOf(start.Name))
			}
		}

		// Assign attributes.
		// Also, determine whether we need to save character data or comments.
		for i := range tinfo.fields {
			finfo := &tinfo.fields[i]
			switch finfo.flags & fMode {
			case fAttr:
				strv := finfo.value(sv)
				// Look for attribute.
				for _, a := range start.Attr {
					if a.Name.Local == finfo.name && (finfo.xmlns == "" || finfo.xmlns == a.Name.Space) {
						if err := p.unmarshalAttr(strv, a); err != nil {
							return err
						}
						break
					}
				}

			case fCharData:
				if !saveData.IsValid() {
					saveData = finfo.value(sv)
				}

			case fComment:
				if !saveComment.IsValid() {
					saveComment = finfo.value(sv)
				}

			case fAny, fAny | fElement:
				if !saveAny.IsValid() {
					saveAny = finfo.value(sv)
				}

			case fInnerXml:
				if !saveXML.IsValid() {
					saveXML = finfo.value(sv)
					if p.saved == nil {
						saveXMLIndex = 0
						p.saved = new(bytes.Buffer)
					} else {
						saveXMLIndex = p.savedOffset()
					}
				}
			}
		}
	}

	// Find end element.
	// Process sub-elements along the way.
Loop:
	for {
		var savedOffset int
		if saveXML.IsValid() {
			savedOffset = p.savedOffset()
		}
		tok, err := p.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case StartElement:
			consumed := false
			if sv.IsValid() {
				consumed, err = p.unmarshalPath(tinfo, sv, nil, &t)
				if err != nil {
					return err
				}
				if !consumed && saveAny.IsValid() {
					consumed = true
					if err := p.unmarshal(saveAny, &t); err != nil {
						return err
					}
				}
			}
			if !consumed {
				if err := p.Skip(); err != nil {
					return err
				}
			}

		case EndElement:
			if saveXML.IsValid() {
				saveXMLData = p.saved.Bytes()[saveXMLIndex:savedOffset]
				if saveXMLIndex == 0 {
					p.saved = nil
				}
			}
			break Loop

		case CharData:
			if saveData.IsValid() {
				data = append(data, t...)
			}

		case Comment:
			if saveComment.IsValid() {
				comment = append(comment, t...)
			}
		}
	}

	if saveData.IsValid() && saveData.CanInterface() && saveData.Type().Implements(textUnmarshalerType) {
		if err := saveData.Interface().(encoding.TextUnmarshaler).UnmarshalText(data); err != nil {
			return err
		}
		saveData = reflect.Value{}
	}

	if saveData.IsValid() && saveData.CanAddr() {
		pv := saveData.Addr()
		if pv.CanInterface() && pv.Type().Implements(textUnmarshalerType) {
			if err := pv.Interface().(encoding.TextUnmarshaler).UnmarshalText(data); err != nil {
				return err
			}
			saveData = reflect.Value{}
		}
	}

	if err := copyValue(saveData, data); err != nil {
		return err
	}

	switch t := saveComment; t.Kind() {
	case reflect.String:
		t.SetString(string(comment))
	case reflect.Slice:
		t.Set(reflect.ValueOf(comment))
	}

	switch t := saveXML; t.Kind() {
	case reflect.String:
		t.SetString(string(saveXMLData))
	case reflect.Slice:
		t.Set(reflect.ValueOf(saveXMLData))
	}

	return nil
}

func copyValue(dst reflect.Value, src []byte) (err error) {
	dst0 := dst

	if dst.Kind() == reflect.Ptr {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		dst = dst.Elem()
	}

	// Save accumulated data.
	switch dst.Kind() {
	case reflect.Invalid:
		// Probably a comment.
	default:
		return errors.New("cannot unmarshal into " + dst0.Type().String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		itmp, err := strconv.ParseInt(string(src), 10, dst.Type().Bits())
		if err != nil {
			return err
		}
		dst.SetInt(itmp)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		var utmp uint64
		if len(src) > 0 && src[0] == '-' {
			// Negative value for unsigned field.
			// Assume it was serialized following two's complement.
			itmp, err := strconv.ParseInt(string(src), 10, dst.Type().Bits())
			if err != nil {
				return err
			}
			// Reinterpret value based on type width.
			switch dst.Type().Bits() {
			case 8:
				utmp = uint64(uint8(itmp))
			case 16:
				utmp = uint64(uint16(itmp))
			case 32:
				utmp = uint64(uint32(itmp))
			case 64:
				utmp = uint64(uint64(itmp))
			}
		} else {
			utmp, err = strconv.ParseUint(string(src), 10, dst.Type().Bits())
			if err != nil {
				return err
			}
		}
		dst.SetUint(utmp)
	case reflect.Float32, reflect.Float64:
		ftmp, err := strconv.ParseFloat(string(src), dst.Type().Bits())
		if err != nil {
			return err
		}
		dst.SetFloat(ftmp)
	case reflect.Bool:
		value, err := strconv.ParseBool(strings.TrimSpace(string(src)))
		if err != nil {
			return err
		}
		dst.SetBool(value)
	case reflect.String:
		dst.SetString(string(src))
	case reflect.Slice:
		if len(src) == 0 {
			// non-nil to flag presence
			src = []byte{}
		}
		dst.SetBytes(src)
	}
	return nil
}

// unmarshalPath walks down an XML structure looking for wanted
// paths, and calls unmarshal on them.
// The consumed result tells whether XML elements have been consumed
// from the Decoder until start's matching end element, or if it's
// still untouched because start is uninteresting for sv's fields.
func (p *Decoder) unmarshalPath(tinfo *typeInfo, sv reflect.Value, parents []string, start *StartElement) (consumed bool, err error) {
	recurse := false
Loop:
	for i := range tinfo.fields {
		finfo := &tinfo.fields[i]
		if finfo.flags&fElement == 0 || len(finfo.parents) < len(parents) || finfo.xmlns != "" && finfo.xmlns != start.Name.Space {
			continue
		}
		for j := range parents {
			if parents[j] != finfo.parents[j] {
				continue Loop
			}
		}
		if len(finfo.parents) == len(parents) && finfo.name == start.Name.Local {
			// It's a perfect match, unmarshal the field.
			return true, p.unmarshal(finfo.value(sv), start)
		}
		if len(finfo.parents) > len(parents) && finfo.parents[len(parents)] == start.Name.Local {
			// It's a prefix for the field. Break and recurse
			// since it's not ok for one field path to be itself
			// the prefix for another field path.
			recurse = true

			// We can reuse the same slice as long as we
			// don't try to append to it.
			parents = finfo.parents[:len(parents)+1]
			break
		}
	}
	if !recurse {
		// We have no business with this element.
		return false, nil
	}
	// The element is not a perfect match for any field, but one
	// or more fields have the path to this element as a parent
	// prefix. Recurse and attempt to match these.
	for {
		var tok Token
		tok, err = p.Token()
		if err != nil {
			return true, err
		}
		switch t := tok.(type) {
		case StartElement:
			consumed2, err := p.unmarshalPath(tinfo, sv, parents, &t)
			if err != nil {
				return true, err
			}
			if !consumed2 {
				if err := p.Skip(); err != nil {
					return true, err
				}
			}
		case EndElement:
			return true, nil
		}
	}
}

// Skip reads tokens until it has consumed the end element
// matching the most recent start element already consumed.
// It recurs if it encounters a start element, so it can be used to
// skip nested structures.
// It returns nil if it finds an end element matching the start
// element; otherwise it returns an error describing the problem.
func (d *Decoder) Skip() error {
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch tok.(type) {
		case StartElement:
			if err := d.Skip(); err != nil {
				return err
			}
		case EndElement:
			return nil
		}
	}
}
