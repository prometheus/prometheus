// +build codegen

package api

import (
	"fmt"
	"strings"
)

// updateTopLevelShapeReferences moves resultWrapper, locationName, and
// xmlNamespace traits from toplevel shape references to the toplevel
// shapes for easier code generation
func (a *API) updateTopLevelShapeReferences() {
	for _, o := range a.Operations {
		// these are for REST-XML services
		if o.InputRef.LocationName != "" {
			o.InputRef.Shape.LocationName = o.InputRef.LocationName
		}
		if o.InputRef.Location != "" {
			o.InputRef.Shape.Location = o.InputRef.Location
		}
		if o.InputRef.Payload != "" {
			o.InputRef.Shape.Payload = o.InputRef.Payload
		}
		if o.InputRef.XMLNamespace.Prefix != "" {
			o.InputRef.Shape.XMLNamespace.Prefix = o.InputRef.XMLNamespace.Prefix
		}
		if o.InputRef.XMLNamespace.URI != "" {
			o.InputRef.Shape.XMLNamespace.URI = o.InputRef.XMLNamespace.URI
		}
	}

}

// writeShapeNames sets each shape's API and shape name values. Binding the
// shape to its parent API. This will set OrigShapeName on each Shape and ShapeRef
// to allow access to the original shape name for code generation.
func (a *API) writeShapeNames() {
	writeOrigShapeName := func(s *ShapeRef) {
		if len(s.ShapeName) > 0 {
			s.OrigShapeName = s.ShapeName
		}
	}

	for n, s := range a.Shapes {
		s.API = a
		s.ShapeName, s.OrigShapeName = n, n
		for _, ref := range s.MemberRefs {
			writeOrigShapeName(ref)
		}
		writeOrigShapeName(&s.MemberRef)
		writeOrigShapeName(&s.KeyRef)
		writeOrigShapeName(&s.ValueRef)
	}
}

func (a *API) resolveReferences() {
	resolver := referenceResolver{API: a, visited: map[*ShapeRef]bool{}}

	for _, s := range a.Shapes {
		resolver.resolveShape(s)
	}

	for _, o := range a.Operations {
		o.API = a // resolve parent reference

		resolver.resolveReference(&o.InputRef)
		resolver.resolveReference(&o.OutputRef)

		// Resolve references for errors also
		for i := range o.ErrorRefs {
			resolver.resolveReference(&o.ErrorRefs[i])
			o.ErrorRefs[i].Shape.Exception = true
			o.ErrorRefs[i].Shape.ErrorInfo.Type = o.ErrorRefs[i].Shape.ShapeName
		}
	}
}

// A referenceResolver provides a way to resolve shape references to
// shape definitions.
type referenceResolver struct {
	*API
	visited map[*ShapeRef]bool
}

// resolveReference updates a shape reference to reference the API and
// its shape definition. All other nested references are also resolved.
func (r *referenceResolver) resolveReference(ref *ShapeRef) {
	if ref.ShapeName == "" {
		return
	}

	shape, ok := r.API.Shapes[ref.ShapeName]
	if !ok {
		panic(fmt.Sprintf("unable resolve reference, %s", ref.ShapeName))
	}

	if ref.JSONValue {
		ref.ShapeName = "JSONValue"
		if _, ok := r.API.Shapes[ref.ShapeName]; !ok {
			r.API.Shapes[ref.ShapeName] = &Shape{
				API:       r.API,
				ShapeName: "JSONValue",
				Type:      "jsonvalue",
				ValueRef: ShapeRef{
					JSONValue: true,
				},
			}
		}
	}

	ref.API = r.API   // resolve reference back to API
	ref.Shape = shape // resolve shape reference

	if r.visited[ref] {
		return
	}
	r.visited[ref] = true

	shape.refs = append(shape.refs, ref) // register the ref

	// resolve shape's references, if it has any
	r.resolveShape(shape)
}

// resolveShape resolves a shape's Member Key Value, and nested member
// shape references.
func (r *referenceResolver) resolveShape(shape *Shape) {
	r.resolveReference(&shape.MemberRef)
	r.resolveReference(&shape.KeyRef)
	r.resolveReference(&shape.ValueRef)
	for _, m := range shape.MemberRefs {
		r.resolveReference(m)
	}
}

// fixStutterNames fixes all name stuttering based on Go naming conventions.
// "Stuttering" is when the prefix of a structure or function matches the
// package name (case insensitive).
func (a *API) fixStutterNames() {
	names, ok := legacyStutterNames[ServiceID(a)]
	if !ok {
		return
	}

	shapeNames := names.ShapeOrder
	if len(shapeNames) == 0 {
		shapeNames = make([]string, 0, len(names.Shapes))
		for k := range names.Shapes {
			shapeNames = append(shapeNames, k)
		}
	}

	for _, shapeName := range shapeNames {
		s := a.Shapes[shapeName]
		newName := names.Shapes[shapeName]
		if other, ok := a.Shapes[newName]; ok && (other.Type == "structure" || other.Type == "enum") {
			panic(fmt.Sprintf(
				"shape name already exists, renaming %v to %v\n",
				s.ShapeName, newName))
		}
		s.Rename(newName)
	}

	for opName, newName := range names.Operations {
		if _, ok := a.Operations[newName]; ok {
			panic(fmt.Sprintf(
				"operation name already exists, renaming %v to %v\n",
				opName, newName))
		}
		op := a.Operations[opName]
		delete(a.Operations, opName)
		a.Operations[newName] = op
		op.ExportedName = newName
	}
}

// renameExportable renames all operation names to be exportable names.
// All nested Shape names are also updated to the exportable variant.
func (a *API) renameExportable() {
	for name, op := range a.Operations {
		newName := a.ExportableName(name)
		if newName != name {
			delete(a.Operations, name)
			a.Operations[newName] = op
		}
		op.ExportedName = newName
	}

	for k, s := range a.Shapes {
		// FIXME SNS has lower and uppercased shape names with the same name,
		// except the lowercased variant is used exclusively for string and
		// other primitive types. Renaming both would cause a collision.
		// We work around this by only renaming the structure shapes.
		if s.Type == "string" {
			continue
		}

		for mName, member := range s.MemberRefs {
			newName := a.ExportableName(mName)
			if newName != mName {
				delete(s.MemberRefs, mName)
				s.MemberRefs[newName] = member

				// also apply locationName trait so we keep the old one
				// but only if there's no locationName trait on ref or shape
				if member.LocationName == "" && member.Shape.LocationName == "" {
					member.LocationName = mName
				}
			}

			if newName == "_" {
				panic("Shape " + s.ShapeName + " uses reserved member name '_'")
			}
		}

		newName := a.ExportableName(k)
		if newName != s.ShapeName {
			s.Rename(newName)
		}

		s.Payload = a.ExportableName(s.Payload)

		// fix required trait names
		for i, n := range s.Required {
			s.Required[i] = a.ExportableName(n)
		}
	}

	for _, s := range a.Shapes {
		// fix enum names
		if s.IsEnum() {
			s.EnumConsts = make([]string, len(s.Enum))
			for i := range s.Enum {
				shape := s.ShapeName
				shape = strings.ToUpper(shape[0:1]) + shape[1:]
				s.EnumConsts[i] = shape + s.EnumName(i)
			}
		}
	}
}

// renameCollidingFields will rename any fields that uses an SDK or Golang
// specific name.
func (a *API) renameCollidingFields() {
	for _, v := range a.Shapes {
		namesWithSet := map[string]struct{}{}
		for k, field := range v.MemberRefs {
			if _, ok := v.MemberRefs["Set"+k]; ok {
				namesWithSet["Set"+k] = struct{}{}
			}

			if collides(k) || (v.Exception && exceptionCollides(k)) {
				renameCollidingField(k, v, field)
			}
		}

		// checks if any field names collide with setters.
		for name := range namesWithSet {
			field := v.MemberRefs[name]
			renameCollidingField(name, v, field)
		}
	}
}

func renameCollidingField(name string, v *Shape, field *ShapeRef) {
	newName := name + "_"
	debugLogger.Logf("Shape %s's field %q renamed to %q", v.ShapeName, name, newName)
	delete(v.MemberRefs, name)
	v.MemberRefs[newName] = field
	// Set LocationName to the original field name if it is not already set.
	// This is to ensure we correctly serialize to the proper member name
	if len(field.LocationName) == 0 {
		field.LocationName = name
	}
}

// collides will return true if it is a name used by the SDK or Golang.
func collides(name string) bool {
	switch name {
	case "String",
		"GoString",
		"Validate":
		return true
	}
	return false
}

func exceptionCollides(name string) bool {
	switch name {
	case "Code",
		"Message",
		"OrigErr":
		return true
	}
	return false
}

func (a *API) applyShapeNameAliases() {
	service, ok := shapeNameAliases[a.name]
	if !ok {
		return
	}

	// Generic Shape Aliases
	for name, s := range a.Shapes {
		if alias, ok := service[name]; ok {
			s.Rename(alias)
			s.AliasedShapeName = true
		}
	}
}

// createInputOutputShapes creates toplevel input/output shapes if they
// have not been defined in the API. This normalizes all APIs to always
// have an input and output structure in the signature.
func (a *API) createInputOutputShapes() {
	for _, op := range a.Operations {
		createAPIParamShape(a, op.Name, &op.InputRef, op.ExportedName+"Input",
			shamelist.Input,
		)
		op.InputRef.Shape.UsedAsInput = true

		createAPIParamShape(a, op.Name, &op.OutputRef, op.ExportedName+"Output",
			shamelist.Output,
		)
		op.OutputRef.Shape.UsedAsOutput = true
	}
}

func (a *API) renameAPIPayloadShapes() {
	for _, op := range a.Operations {
		op.InputRef.Payload = a.ExportableName(op.InputRef.Payload)
		op.OutputRef.Payload = a.ExportableName(op.OutputRef.Payload)
	}
}

func createAPIParamShape(a *API, opName string, ref *ShapeRef, shapeName string, shamelistLookup func(string, string) bool) {
	if len(ref.ShapeName) == 0 {
		setAsPlacholderShape(ref, shapeName, a)
		return
	}

	// nothing to do if already the correct name.
	if s := ref.Shape; s.AliasedShapeName || s.ShapeName == shapeName || shamelistLookup(a.name, opName) {
		return
	}

	if s, ok := a.Shapes[shapeName]; ok {
		panic(fmt.Sprintf(
			"attempting to create duplicate API parameter shape, %v, %v, %v, %v\n",
			shapeName, opName, ref.ShapeName, s.OrigShapeName,
		))
	}

	ref.Shape.removeRef(ref)
	ref.ShapeName = shapeName
	ref.Shape = ref.Shape.Clone(shapeName)
	ref.Shape.refs = append(ref.Shape.refs, ref)
}

func setAsPlacholderShape(tgtShapeRef *ShapeRef, name string, a *API) {
	shape := a.makeIOShape(name)
	shape.Placeholder = true
	*tgtShapeRef = ShapeRef{API: a, ShapeName: shape.ShapeName, Shape: shape}
	shape.refs = append(shape.refs, tgtShapeRef)
}

// makeIOShape returns a pointer to a new Shape initialized by the name provided.
func (a *API) makeIOShape(name string) *Shape {
	shape := &Shape{
		API: a, ShapeName: name, Type: "structure",
		MemberRefs: map[string]*ShapeRef{},
	}
	a.Shapes[name] = shape
	return shape
}

// removeUnusedShapes removes shapes from the API which are not referenced by
// any other shape in the API.
func (a *API) removeUnusedShapes() {
	for _, s := range a.Shapes {
		if len(s.refs) == 0 {
			a.removeShape(s)
		}
	}
}

// Represents the service package name to EndpointsID mapping
var custEndpointsKey = map[string]string{
	"applicationautoscaling": "application-autoscaling",
}

// Sents the EndpointsID field of Metadata  with the value of the
// EndpointPrefix if EndpointsID is not set. Also adds
// customizations for services if EndpointPrefix is not a valid key.
func (a *API) setMetadataEndpointsKey() {
	if len(a.Metadata.EndpointsID) != 0 {
		return
	}

	if v, ok := custEndpointsKey[a.PackageName()]; ok {
		a.Metadata.EndpointsID = v
	} else {
		a.Metadata.EndpointsID = a.Metadata.EndpointPrefix
	}
}

func (a *API) findEndpointDiscoveryOp() {
	for _, op := range a.Operations {
		if op.IsEndpointDiscoveryOp {
			a.EndpointDiscoveryOp = op
			return
		}
	}
}
func (a *API) injectUnboundedOutputStreaming() {
	for _, op := range a.Operations {
		if op.AuthType != V4UnsignedBodyAuthType {
			continue
		}
		for _, ref := range op.InputRef.Shape.MemberRefs {
			if ref.Streaming || ref.Shape.Streaming {
				if len(ref.Documentation) != 0 {
					ref.Documentation += `
//`
				}
				ref.Documentation += `
// To use an non-seekable io.Reader for this request wrap the io.Reader with
// "aws.ReadSeekCloser". The SDK will not retry request errors for non-seekable
// readers. This will allow the SDK to send the reader's payload as chunked
// transfer encoding.`
			}
		}
	}
}
