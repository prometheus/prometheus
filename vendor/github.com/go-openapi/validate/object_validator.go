// Copyright 2015 go-swagger maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package validate

import (
	"reflect"
	"regexp"
	"strings"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/spec"
	"github.com/go-openapi/strfmt"
)

type objectValidator struct {
	Path                 string
	In                   string
	MaxProperties        *int64
	MinProperties        *int64
	Required             []string
	Properties           map[string]spec.Schema
	AdditionalProperties *spec.SchemaOrBool
	PatternProperties    map[string]spec.Schema
	Root                 interface{}
	KnownFormats         strfmt.Registry
	Options              SchemaValidatorOptions
}

func (o *objectValidator) SetPath(path string) {
	o.Path = path
}

func (o *objectValidator) Applies(source interface{}, kind reflect.Kind) bool {
	// TODO: this should also work for structs
	// there is a problem in the type validator where it will be unhappy about null values
	// so that requires more testing
	r := reflect.TypeOf(source) == specSchemaType && (kind == reflect.Map || kind == reflect.Struct)
	debugLog("object validator for %q applies %t for %T (kind: %v)\n", o.Path, r, source, kind)
	return r
}

func (o *objectValidator) isProperties() bool {
	p := strings.Split(o.Path, ".")
	return len(p) > 1 && p[len(p)-1] == jsonProperties && p[len(p)-2] != jsonProperties
}

func (o *objectValidator) isDefault() bool {
	p := strings.Split(o.Path, ".")
	return len(p) > 1 && p[len(p)-1] == jsonDefault && p[len(p)-2] != jsonDefault
}

func (o *objectValidator) isExample() bool {
	p := strings.Split(o.Path, ".")
	return len(p) > 1 && (p[len(p)-1] == swaggerExample || p[len(p)-1] == swaggerExamples) && p[len(p)-2] != swaggerExample
}

func (o *objectValidator) checkArrayMustHaveItems(res *Result, val map[string]interface{}) {
	// for swagger 2.0 schemas, there is an additional constraint to have array items defined explicitly.
	// with pure jsonschema draft 4, one may have arrays with undefined items (i.e. any type).
	if t, typeFound := val[jsonType]; typeFound {
		if tpe, ok := t.(string); ok && tpe == arrayType {
			if item, itemsKeyFound := val[jsonItems]; !itemsKeyFound {
				res.AddErrors(errors.Required(jsonItems, o.Path, item))
			}
		}
	}
}

func (o *objectValidator) checkItemsMustBeTypeArray(res *Result, val map[string]interface{}) {
	if !o.isProperties() && !o.isDefault() && !o.isExample() {
		if _, itemsKeyFound := val[jsonItems]; itemsKeyFound {
			t, typeFound := val[jsonType]
			if typeFound {
				if tpe, ok := t.(string); !ok || tpe != arrayType {
					res.AddErrors(errors.InvalidType(o.Path, o.In, arrayType, nil))
				}
			} else {
				// there is no type
				res.AddErrors(errors.Required(jsonType, o.Path, t))
			}
		}
	}
}

func (o *objectValidator) precheck(res *Result, val map[string]interface{}) {
	if o.Options.EnableArrayMustHaveItemsCheck {
		o.checkArrayMustHaveItems(res, val)
	}
	if o.Options.EnableObjectArrayTypeCheck {
		o.checkItemsMustBeTypeArray(res, val)
	}
}

func (o *objectValidator) Validate(data interface{}) *Result {
	val := data.(map[string]interface{})
	// TODO: guard against nil data
	numKeys := int64(len(val))

	if o.MinProperties != nil && numKeys < *o.MinProperties {
		return errorHelp.sErr(errors.TooFewProperties(o.Path, o.In, *o.MinProperties))
	}
	if o.MaxProperties != nil && numKeys > *o.MaxProperties {
		return errorHelp.sErr(errors.TooManyProperties(o.Path, o.In, *o.MaxProperties))
	}

	res := new(Result)

	o.precheck(res, val)

	// check validity of field names
	if o.AdditionalProperties != nil && !o.AdditionalProperties.Allows {
		// Case: additionalProperties: false
		for k := range val {
			_, regularProperty := o.Properties[k]
			matched := false

			for pk := range o.PatternProperties {
				if matches, _ := regexp.MatchString(pk, k); matches {
					matched = true
					break
				}
			}

			if !regularProperty && k != "$schema" && k != "id" && !matched {
				// Special properties "$schema" and "id" are ignored
				res.AddErrors(errors.PropertyNotAllowed(o.Path, o.In, k))

				// BUG(fredbi): This section should move to a part dedicated to spec validation as
				// it will conflict with regular schemas where a property "headers" is defined.

				//
				// Croaks a more explicit message on top of the standard one
				// on some recognized cases.
				//
				// NOTE: edge cases with invalid type assertion are simply ignored here.
				// NOTE: prefix your messages here by "IMPORTANT!" so there are not filtered
				// by higher level callers (the IMPORTANT! tag will be eventually
				// removed).
				if k == "headers" && val[k] != nil {
					// $ref is forbidden in header
					if headers, mapOk := val[k].(map[string]interface{}); mapOk {
						for headerKey, headerBody := range headers {
							if headerBody != nil {
								if headerSchema, mapOfMapOk := headerBody.(map[string]interface{}); mapOfMapOk {
									if _, found := headerSchema["$ref"]; found {
										var msg string
										if refString, stringOk := headerSchema["$ref"].(string); stringOk {
											msg = strings.Join([]string{", one may not use $ref=\":", refString, "\""}, "")
										}
										res.AddErrors(refNotAllowedInHeaderMsg(o.Path, headerKey, msg))
									}
								}
							}
						}
					}
					/*
						case "$ref":
							if val[k] != nil {
								// TODO: check context of that ref: warn about siblings, check against invalid context
							}
					*/
				}
			}
		}
	} else {
		// Cases: no additionalProperties (implying: true), or additionalProperties: true, or additionalProperties: { <<schema>> }
		for key, value := range val {
			_, regularProperty := o.Properties[key]

			// Validates property against "patternProperties" if applicable
			// BUG(fredbi): succeededOnce is always false

			// NOTE: how about regular properties which do not match patternProperties?
			matched, succeededOnce, _ := o.validatePatternProperty(key, value, res)

			if !(regularProperty || matched || succeededOnce) {

				// Cases: properties which are not regular properties and have not been matched by the PatternProperties validator
				if o.AdditionalProperties != nil && o.AdditionalProperties.Schema != nil {
					// AdditionalProperties as Schema
					r := NewSchemaValidator(o.AdditionalProperties.Schema, o.Root, o.Path+"."+key, o.KnownFormats, o.Options.Options()...).Validate(value)
					res.mergeForField(data.(map[string]interface{}), key, r)
				} else if regularProperty && !(matched || succeededOnce) {
					// TODO: this is dead code since regularProperty=false here
					res.AddErrors(errors.FailedAllPatternProperties(o.Path, o.In, key))
				}
			}
		}
		// Valid cases: additionalProperties: true or undefined
	}

	createdFromDefaults := map[string]bool{}

	// Property types:
	// - regular Property
	for pName := range o.Properties {
		pSchema := o.Properties[pName] // one instance per iteration
		rName := pName
		if o.Path != "" {
			rName = o.Path + "." + pName
		}

		// Recursively validates each property against its schema
		if v, ok := val[pName]; ok {
			r := NewSchemaValidator(&pSchema, o.Root, rName, o.KnownFormats, o.Options.Options()...).Validate(v)
			res.mergeForField(data.(map[string]interface{}), pName, r)
		} else if pSchema.Default != nil {
			// If a default value is defined, creates the property from defaults
			// NOTE: JSON schema does not enforce default values to be valid against schema. Swagger does.
			createdFromDefaults[pName] = true
			res.addPropertySchemata(data.(map[string]interface{}), pName, &pSchema)
		}
	}

	// Check required properties
	if len(o.Required) > 0 {
		for _, k := range o.Required {
			if v, ok := val[k]; !ok && !createdFromDefaults[k] {
				res.AddErrors(errors.Required(o.Path+"."+k, o.In, v))
				continue
			}
		}
	}

	// Check patternProperties
	// TODO: it looks like we have done that twice in many cases
	for key, value := range val {
		_, regularProperty := o.Properties[key]
		matched, _ /*succeededOnce*/, patterns := o.validatePatternProperty(key, value, res)
		if !regularProperty && (matched /*|| succeededOnce*/) {
			for _, pName := range patterns {
				if v, ok := o.PatternProperties[pName]; ok {
					r := NewSchemaValidator(&v, o.Root, o.Path+"."+key, o.KnownFormats, o.Options.Options()...).Validate(value)
					res.mergeForField(data.(map[string]interface{}), key, r)
				}
			}
		}
	}
	return res
}

// TODO: succeededOnce is not used anywhere
func (o *objectValidator) validatePatternProperty(key string, value interface{}, result *Result) (bool, bool, []string) {
	matched := false
	succeededOnce := false
	var patterns []string

	for k, schema := range o.PatternProperties {
		sch := schema
		if match, _ := regexp.MatchString(k, key); match {
			patterns = append(patterns, k)
			matched = true
			validator := NewSchemaValidator(&sch, o.Root, o.Path+"."+key, o.KnownFormats, o.Options.Options()...)

			res := validator.Validate(value)
			result.Merge(res)
		}
	}

	// BUG(fredbi): can't get to here. Should remove dead code (commented out).

	// if succeededOnce {
	//	result.Inc()
	// }

	return matched, succeededOnce, patterns
}
